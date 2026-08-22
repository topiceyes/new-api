import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { get as httpGet } from 'node:http'

import { defineConfig, loadEnv } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { pluginTailwindcss } from '@rsbuild/plugin-tailwindcss'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

// Dev-server mirror of the backend's RenderIndexPage: the production binary
// rewrites the hardcoded title/meta to the admin-configured system name at
// serve time, and this middleware does the same for `bun run dev` responses
// so the raw "New API" template string is never served on the wire.
const escapeHtml = (value: string) =>
  value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

export default defineConfig(({ envMode }) => {
  const env = loadEnv({ mode: envMode, prefixes: ['VITE_'] })
  const serverUrl =
    process.env.VITE_REACT_APP_SERVER_URL ||
    env.rawPublicVars.VITE_REACT_APP_SERVER_URL ||
    'http://localhost:3000'

  // Short TTL: admins renaming the system should see dev responses follow
  // within seconds, without hammering /api/status on every page load.
  const SYSTEM_NAME_CACHE_TTL_MS = 5000
  let resolvedSystemName: { value: string; expiresAt: number } | undefined
  // Node's fetch (undici) refuses "bad ports" like 6000, so use node:http.
  const fetchSystemName = (): Promise<string | undefined> =>
    new Promise((resolve) => {
      const req = httpGet(`${serverUrl}/api/status`, (res) => {
        const chunks: Buffer[] = []
        res.on('data', (chunk: Buffer) => chunks.push(chunk))
        res.on('end', () => {
          try {
            const json = JSON.parse(Buffer.concat(chunks).toString('utf8')) as {
              data?: { system_name?: unknown }
            }
            const name = json?.data?.system_name
            resolve(typeof name === 'string' && name.trim() ? name : undefined)
          } catch {
            resolve(undefined)
          }
        })
      })
      req.on('error', () => resolve(undefined))
      req.setTimeout(3000, () => req.destroy())
    })
  const resolveSystemName = async (): Promise<string> => {
    if (resolvedSystemName && resolvedSystemName.expiresAt > Date.now()) {
      return resolvedSystemName.value
    }
    const name = await fetchSystemName()
    // Cache only on success so a request before the backend is up retries.
    if (name) {
      resolvedSystemName = {
        value: name,
        expiresAt: Date.now() + SYSTEM_NAME_CACHE_TTL_MS,
      }
    }
    return name ?? resolvedSystemName?.value ?? 'New API'
  }

  const isProd = envMode === 'production'
  const devProxy = Object.fromEntries(
    (['/api', '/mj', '/pg'] as const).map((key) => [
      key,
      { target: serverUrl, changeOrigin: true },
    ])
  ) as Record<string, { target: string; changeOrigin: boolean }>

  return {
    plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
    // Rsbuild 2: replaces deprecated `performance.chunkSplit` (RSPack 2 aligned)
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-ui-primitives': {
          test: /node_modules[\\/](@base-ui|@radix-ui)[\\/]/,
          name: 'vendor-ui-primitives',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: {
        index: './src/main.tsx',
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    html: {
      template: './index.html',
    },
    server: {
      host: '0.0.0.0',
      strictPort: false,
      proxy: devProxy,
      setup: [
        ({ server, action }) => {
          if (action !== 'dev') return
          // Registered before built-ins, so the raw template is never served.
          server.middlewares.use((req, res, next) => {
            const pathname = req.url?.split('?')[0]
            if (
              req.method !== 'GET' ||
              (pathname !== '/' && pathname !== '/index.html')
            ) {
              return next()
            }
            // This middleware mutates the body, which corrupts any compressed
            // stream (browser: ERR_CONTENT_DECODING_FAILED, blank page; curl
            // sends no Accept-Encoding so it keeps working). Disable downstream
            // compression for these two document requests — dev-only, so the
            // overhead is irrelevant.
            delete req.headers['accept-encoding']
            // Buffer the HTML response, swap the title/meta, then flush.
            const chunks: Buffer[] = []
            const originalEnd = res.end.bind(res)
            res.write = ((chunk: unknown) => {
              if (chunk != null && typeof chunk !== 'function') {
                chunks.push(
                  Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk))
                )
              }
              return true
            }) as typeof res.write
            res.end = ((chunk: unknown, ...rest: unknown[]) => {
              if (chunk != null && typeof chunk !== 'function') {
                chunks.push(
                  Buffer.isBuffer(chunk) ? chunk : Buffer.from(String(chunk))
                )
              }
              const flush = (body: Buffer) => {
                if (!res.headersSent) {
                  // Defensive: with accept-encoding removed nothing downstream
                  // should set these, but a stale header would corrupt the body.
                  res.removeHeader('content-encoding')
                  res.setHeader('content-length', body.length)
                }
                originalEnd(body, ...(rest as []))
              }
              resolveSystemName()
                .then((name) => {
                  const escaped = escapeHtml(name)
                  const html = Buffer.concat(chunks)
                    .toString('utf8')
                    .split('<title>New API</title>')
                    .join(`<title>${escaped}</title>`)
                    .split('<meta name="title" content="New API" />')
                    .join(`<meta name="title" content="${escaped}" />`)
                    // Matches the backend RenderIndexPage description swap; the
                    // template keeps the meta tag multi-line, so only the
                    // content attribute value is a stable anchor.
                    .split('content="Unified AI API gateway and admin dashboard."')
                    .join(`content="${escaped}"`)
                  flush(Buffer.from(html, 'utf8'))
                })
                .catch(() => flush(Buffer.concat(chunks)))
              return res
            }) as typeof res.end
            next()
          })
        },
      ],
    },
    output: {
      // Production optimizations
      minify: isProd,
      target: 'web',
      distPath: {
        root: 'dist',
      },
      // Rely on Rsbuild default legalComments ("linked" → per-chunk *.LICENSE.txt) in all modes.
      // Do not set "none" in production: that strips minifier-preserved third-party notices and
      // extracted license files, which some distributions require for open-source compliance.
    },
    performance: {
      // Remove console in production
      removeConsole: isProd ? ['log'] : false,
      buildCache: false,
    },
    tools: {
      rspack: {
        plugins: [
          tanstackRouter({
            target: 'react',
            // Dev: avoid per-route async chunks (reduces white flash on navigation + faster HMR feedback).
            // Prod: keep route-based code splitting.
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})
