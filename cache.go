package mycache

import (
	"mycache/lru"
	"sync"
)
//带锁的缓存器
type cache struct {
	mu sync.Mutex
	lru *lru.Cache
	cacheBytes int64 //最大容量
}

func (c *cache)add(key string,val ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru==nil{ //延迟初始化(Lazy Initialization)
		c.lru=lru.New(c.cacheBytes,nil)
	}
	c.lru.Add(key,val)
}

func (c *cache)get(key string) (val ByteView,ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru==nil{
		return
	}
	if ans,ok:=c.lru.Get(key);ok{
		return ans.(ByteView),ok
	}
	return
}