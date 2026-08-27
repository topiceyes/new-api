package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/go-redis/redis/v8"
)

// 多 Key 渠道「令牌粘性 Key 绑定」:同一令牌的请求固定落到同一个上游 key,
// 空闲超过配置时间自动释放供其他令牌绑定。目的是让每个 key 的并发/话题/时段
// 画像接近单客户端,降低被上游风控识别为共享网关的概率。
//
// 绑定是易失状态,只在 Redis(多实例)或进程内存(单实例)里维护,不落库。
// 双向结构:
//   sticky:ch:{channelId}:t:{tokenId} -> keyIdx   (令牌 -> key,含共享绑定)
//   sticky:ch:{channelId}:k:{keyIdx}   -> tokenId (key 的独占属主)
// share 策略只写 t 方向,不抢占独占表。

const stickyDefaultIdleMinutes = 10

// StickyBinding 管理端绑定列表的单条快照。
type StickyBinding struct {
	TokenId      int  `json:"token_id"`
	KeyIndex     int  `json:"key_index"`
	Exclusive    bool `json:"exclusive"`
	TtlRemaining int  `json:"ttl_remaining_seconds"`
	IdleSeconds  int  `json:"idle_seconds"`
}

// stickyClockNow 时间 seam,测试注入虚拟时钟。
var stickyClockNow = time.Now

func stickyIdleDuration(idleMinutes int) time.Duration {
	if idleMinutes <= 0 {
		idleMinutes = stickyDefaultIdleMinutes
	}
	return time.Duration(idleMinutes) * time.Minute
}

func stickyTokenKey(channelId, tokenId int) string {
	return fmt.Sprintf("sticky:ch:%d:t:%d", channelId, tokenId)
}

func stickyOwnerKey(channelId, keyIdx int) string {
	return fmt.Sprintf("sticky:ch:%d:k:%d", channelId, keyIdx)
}

// stickyBindLua 原子独占绑定:若令牌已有绑定直接返回(防 Go 侧检查与写入间的
// 并发窗口);否则按候选顺序找第一个无主 key,双向 SET + EXPIRE,返回 keyIdx。
// 全部被占返回 -1。KEYS[1]=t-key, KEYS[2..]=候选 k-key;ARGV[1]=ttl秒,
// ARGV[2]=tokenId, ARGV[3..]=与 KEYS[2..] 对应的 keyIdx。
var stickyBindLua = `
local tb = redis.call('GET', KEYS[1])
if tb then return tonumber(tb) end
for i = 2, #KEYS do
	if redis.call('GET', KEYS[i]) == false then
		redis.call('SET', KEYS[i], ARGV[2], 'EX', tonumber(ARGV[1]))
		redis.call('SET', KEYS[1], ARGV[i + 1], 'EX', tonumber(ARGV[1]))
		return tonumber(ARGV[i + 1])
	end
end
return -1
`

// BindTokenToChannelKey 为令牌选取/复用绑定 key。enabledIdx 是当前启用 key 的
// 下标列表(空列表由调用方在进入前拦截)。ok=false 表示 key 全被占用且策略为
// busy;allowShare=true 时全占则随机共用一个 key,永不返回 ok=false。
// Redis 故障时降级进程内存并记日志(单实例语义,同 keyshare 约定)。
func BindTokenToChannelKey(tokenId, channelId int, enabledIdx []int, idleMinutes int, allowShare bool) (int, bool) {
	idle := stickyIdleDuration(idleMinutes)
	if common.RedisEnabled {
		idx, ok, err := bindTokenRedis(tokenId, channelId, enabledIdx, idle, allowShare)
		if err == nil {
			return idx, ok
		}
		common.SysError("sticky key binding: redis failed, degrade to memory: " + err.Error())
	}
	return bindTokenMemory(tokenId, channelId, enabledIdx, idle, allowShare)
}

func bindTokenRedis(tokenId, channelId int, enabledIdx []int, idle time.Duration, allowShare bool) (int, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tKey := stickyTokenKey(channelId, tokenId)
	enabledSet := make(map[int]bool, len(enabledIdx))
	for _, idx := range enabledIdx {
		enabledSet[idx] = true
	}

	// 快路径:已有绑定且 key 仍有效 -> 刷新 TTL 复用。
	if v, err := common.RDB.Get(ctx, tKey).Result(); err == nil {
		if idx, perr := strconv.Atoi(v); perr == nil && enabledSet[idx] {
			pipe := common.RDB.TxPipeline()
			pipe.Expire(ctx, tKey, idle)
			if owner, oerr := common.RDB.Get(ctx, stickyOwnerKey(channelId, idx)).Result(); oerr == nil && owner == strconv.Itoa(tokenId) {
				pipe.Expire(ctx, stickyOwnerKey(channelId, idx), idle)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				return 0, false, err
			}
			return idx, true, nil
		}
		// 绑定的 key 已禁用/越界(管理员删了 key):清掉重绑。
		if idx, perr := strconv.Atoi(v); perr == nil {
			common.RDB.Del(ctx, tKey, stickyOwnerKey(channelId, idx))
		} else {
			common.RDB.Del(ctx, tKey)
		}
	} else if err != nil && !errors.Is(err, redis.Nil) {
		return 0, false, err
	}
	keys := make([]string, 0, len(enabledIdx)+1)
	args := make([]any, 0, len(enabledIdx)+2)
	keys = append(keys, tKey)
	args = append(args, int(idle.Seconds()), strconv.Itoa(tokenId))
	for _, idx := range enabledIdx {
		keys = append(keys, stickyOwnerKey(channelId, idx))
		args = append(args, idx)
	}

	res, err := common.RDB.Eval(ctx, stickyBindLua, keys, args...).Result()
	if err != nil {
		return 0, false, err
	}
	bound, _ := res.(int64)
	// Lua 快路径可能返回了未校验启用状态的老绑定,失效则清理后重试一次。
	if bound >= 0 && !enabledSet[int(bound)] {
		common.RDB.Del(ctx, tKey, stickyOwnerKey(channelId, int(bound)))
		res2, err2 := common.RDB.Eval(ctx, stickyBindLua, keys, args...).Result()
		if err2 != nil {
			return 0, false, err2
		}
		bound, _ = res2.(int64)
	}
	if bound < 0 {
		return stickyShareRedis(ctx, tKey, enabledIdx, idle, allowShare)
	}
	// Lua 快路径命中老绑定时未刷新 TTL,这里统一补上。
	pipe := common.RDB.TxPipeline()
	pipe.Expire(ctx, tKey, idle)
	pipe.Expire(ctx, stickyOwnerKey(channelId, int(bound)), idle)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, false, err
	}
	return int(bound), true, nil
}

// stickyShareRedis 全占时的 share 策略:随机挑一个启用 key 单向绑定(不占独占表)。
func stickyShareRedis(ctx context.Context, tKey string, enabledIdx []int, idle time.Duration, allowShare bool) (int, bool, error) {
	if !allowShare || len(enabledIdx) == 0 {
		return -1, false, nil
	}
	pick := enabledIdx[rand.Intn(len(enabledIdx))]
	if err := common.RDB.Set(ctx, tKey, strconv.Itoa(pick), idle).Err(); err != nil {
		return 0, false, err
	}
	return pick, true, nil
}

// ListChannelStickyBindings 返回渠道当前的粘性绑定快照(含共享绑定),供管理端列表。
func ListChannelStickyBindings(channelId int, idleMinutes int) []StickyBinding {
	idle := stickyIdleDuration(idleMinutes)
	if common.RedisEnabled {
		if list, err := listStickyRedis(channelId, idle); err == nil {
			return list
		}
	}
	return listStickyMemory(channelId, idle)
}

// ReleaseChannelStickyBinding 手动解绑(仅清理该令牌自己占用的独占项)。
func ReleaseChannelStickyBinding(channelId, tokenId int) {
	if common.RedisEnabled {
		if ok := releaseStickyRedis(channelId, tokenId); ok {
			return
		}
	}
	releaseStickyMemory(channelId, tokenId)
}

// ---------------------------------------------------------------- 内存路径

type stickyChannelBindings struct {
	mu         sync.Mutex
	tokenToIdx map[int]int   // 令牌 -> key 下标(含共享)
	idxToToken map[int]int   // key 下标 -> 独占属主令牌
	lastActive map[int]int64 // 令牌 -> 最近活跃秒
}

var stickyMem sync.Map // channelId -> *stickyChannelBindings

func stickyChannel(channelId int) *stickyChannelBindings {
	v, _ := stickyMem.LoadOrStore(channelId, &stickyChannelBindings{
		tokenToIdx: make(map[int]int),
		idxToToken: make(map[int]int),
		lastActive: make(map[int]int64),
	})
	return v.(*stickyChannelBindings)
}

func (b *stickyChannelBindings) pruneExpired(now int64, idle time.Duration) {
	idleSec := int64(idle.Seconds())
	for t, last := range b.lastActive {
		if now-last >= idleSec {
			idx := b.tokenToIdx[t]
			delete(b.tokenToIdx, t)
			delete(b.lastActive, t)
			if b.idxToToken[idx] == t {
				delete(b.idxToToken, idx)
			}
		}
	}
}

func bindTokenMemory(tokenId, channelId int, enabledIdx []int, idle time.Duration, allowShare bool) (int, bool) {
	if len(enabledIdx) == 0 {
		// 无可用候选(空集上无法随机),由调用方兜底。
		return -1, false
	}
	b := stickyChannel(channelId)
	now := stickyClockNow().Unix()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneExpired(now, idle)

	enabledSet := make(map[int]bool, len(enabledIdx))
	for _, idx := range enabledIdx {
		enabledSet[idx] = true
	}

	if idx, ok := b.tokenToIdx[tokenId]; ok {
		if enabledSet[idx] {
			b.lastActive[tokenId] = now
			return idx, true
		}
		delete(b.tokenToIdx, tokenId)
		delete(b.lastActive, tokenId)
		if b.idxToToken[idx] == tokenId {
			delete(b.idxToToken, idx)
		}
	}

	for _, idx := range enabledIdx {
		if _, taken := b.idxToToken[idx]; !taken {
			b.tokenToIdx[tokenId] = idx
			b.idxToToken[idx] = tokenId
			b.lastActive[tokenId] = now
			return idx, true
		}
	}
	if !allowShare {
		return -1, false
	}
	pick := enabledIdx[rand.Intn(len(enabledIdx))]
	b.tokenToIdx[tokenId] = pick
	b.lastActive[tokenId] = now
	return pick, true
}

func listStickyMemory(channelId int, idle time.Duration) []StickyBinding {
	b := stickyChannel(channelId)
	now := stickyClockNow().Unix()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneExpired(now, idle)

	list := make([]StickyBinding, 0, len(b.tokenToIdx))
	for t, idx := range b.tokenToIdx {
		idleSec := int(now - b.lastActive[t])
		list = append(list, StickyBinding{
			TokenId:      t,
			KeyIndex:     idx,
			Exclusive:    b.idxToToken[idx] == t,
			IdleSeconds:  idleSec,
			TtlRemaining: int(idle.Seconds()) - idleSec,
		})
	}
	return list
}

func releaseStickyMemory(channelId, tokenId int) {
	b := stickyChannel(channelId)
	b.mu.Lock()
	defer b.mu.Unlock()
	idx, ok := b.tokenToIdx[tokenId]
	if !ok {
		return
	}
	delete(b.tokenToIdx, tokenId)
	delete(b.lastActive, tokenId)
	if b.idxToToken[idx] == tokenId {
		delete(b.idxToToken, idx)
	}
}

// ---------------------------------------------------------------- Redis 列表/解绑

func listStickyRedis(channelId int, idle time.Duration) ([]StickyBinding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pattern := fmt.Sprintf("sticky:ch:%d:t:*", channelId)
	seen := make(map[string]bool)
	var list []StickyBinding
	var cursor uint64
	for {
		keys, next, err := common.RDB.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		for _, tKey := range keys {
			if seen[tKey] {
				continue
			}
			seen[tKey] = true
			parts := strings.Split(tKey, ":")
			if len(parts) < 2 {
				continue
			}
			tokenId, err := strconv.Atoi(parts[len(parts)-1])
			if err != nil {
				continue
			}
			v, err := common.RDB.Get(ctx, tKey).Result()
			if err != nil {
				continue
			}
			idx, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			ttl, err := common.RDB.TTL(ctx, tKey).Result()
			if err != nil {
				continue
			}
			owner, _ := common.RDB.Get(ctx, stickyOwnerKey(channelId, idx)).Result()
			idleSec := int(idle.Seconds() - ttl.Seconds())
			list = append(list, StickyBinding{
				TokenId:      tokenId,
				KeyIndex:     idx,
				Exclusive:    owner == strconv.Itoa(tokenId),
				IdleSeconds:  idleSec,
				TtlRemaining: int(ttl.Seconds()),
			})
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return list, nil
}

func releaseStickyRedis(channelId, tokenId int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tKey := stickyTokenKey(channelId, tokenId)
	v, err := common.RDB.Get(ctx, tKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return false // Redis 故障,调用方降级内存
	}
	if err == nil {
		if idx, perr := strconv.Atoi(v); perr == nil {
			if owner, oerr := common.RDB.Get(ctx, stickyOwnerKey(channelId, idx)).Result(); oerr == nil && owner == strconv.Itoa(tokenId) {
				common.RDB.Del(ctx, stickyOwnerKey(channelId, idx))
			}
		}
		common.RDB.Del(ctx, tKey)
	}
	return true
}
