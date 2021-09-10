package mycache

import (
	"fmt"
	"log"
	"sync"
	"mycache/singleflight"
	pb "mycache/mycachepb"
)

/*
使用方法：
（1）构造用户自定义的回调函数getter。用户对缓存进行设置，均需要通过getter。
（2）使用NewGroup，新建一个Group，同时设置：名称name,最大容量cacheBytes,回调函数getter。
（3）新建一个HTTPPool，并使用Group.RegisterPeers将该HTTPPool设置为Group.peers。
（4）使用GetGroup(name)可以获得该name对应的Group的指针。
（5）使用Group.Get(key)即可获得键值对应的value。
 */

type Getter interface {
	Get(key string) ([]byte, error)
}

// GetterFunc是一个回调函数(callback)，在缓存不存在时，调用这个函数，得到源数据。
// 定义一个函数类型 F，并且实现接口 A 的方法，然后在这个方法中调用自己。
// 这是 Go 语言中将其他函数（参数返回值定义与 F 一致）转换为接口 A 的常用技巧。
// GetterFunc类型的函数，均有名为Get的method，因此任意GetterFunc类型的函数都的Getter的实现
type GetterFunc func(key string) ([]byte, error)


// Get implements Getter interface function
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}
//一个 Group 可以认为是一个缓存的命名空间，每个 Group 拥有一个唯一的名称 name。
//比如可以创建三个 Group，缓存学生的成绩命名为 scores，缓存学生信息的命名为 info，
//缓存学生课程的命名为 courses。
// 从Group中进行查找的顺序：（1）查找mainCache；如果命中直接返回value；如果失败则进行（2）
// （2）查找key对应真实节点的名称。如果对应真实节点就是本节点，则进行（3）；如果是其他节点则进行（4）
// （3）通过getter获得key对应value，进行相关处理再返回。
// （4）请求其他节点返回结果。
//
type Group struct {
	name      string
	getter    Getter //第二个属性是 getter Getter，即缓存未命中时获取源数据的回调(callback)。getter是用户自设置的一个函数，用于设置key-value的实际情况。
	mainCache cache  //第三个属性是 mainCache cache，即并发缓存。
	peers     PeerPicker //第四个属性peers是【数据获得器的选择器】，本框架目前仅实现了基于HTTP的节点通信，故peers就是一个HTTPPool。
	//Group中，全部与其他远程节点通信的功能（如发送请求、接收请求）都由PeerPicker（被HTTPPool实现）来负责。
	loader *singleflight.GroupCall
}

var (
	mu sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
		loader:    &singleflight.GroupCall{},
	}
	groups[name] = g
	return g
}

// RegisterPeers registers a PeerPicker for choosing remote peer
// 为Group设置HTTPPool。
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

func GetGroup(name string) *Group {
	//GetGroup 用来特定名称的 Group，这里使用了只读锁 RLock()，因为不涉及任何冲突变量的写操作。
	mu.RLock()
	g := groups[name] //groups指向的是指针，如果键对应的值不存在则返回nil
	mu.RUnlock()
	return g
}

func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}

	if v, ok := g.mainCache.get(key); ok {
		log.Println("[myCache] hit")
		return v, nil
	}
	// 如果存在，即返回。
	// 如果不存在，即导入（load）。
	return g.load(key)
}


// 使用 PickPeer() 方法选择节点，若非本机节点，则调用 getFromPeer() 从远程获取。
// 若是本机节点或失败，则回退到 getLocally()。
func (g *Group) load(key string) (value ByteView, err error) {
	// each key is only fetched once (either locally or remotely)
	// regardless of the number of concurrent callers.
	viewi, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickPeer(key); ok { //如果按一致性哈希该key应该由本节点储存则ok为false。
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[myCache] Failed to get from peer", err)
			}
		}

		return g.getLocally(key)
	})

	if err == nil {
		return viewi.(ByteView), nil
	}
	return


	/*if g.peers != nil {
		if peer, ok := g.peers.PickPeer(key); ok { //如果按一致性哈希该key应该由本节点储存则ok为false。
			if value, err = g.getFromPeer(peer, key); err == nil {
				return value, nil
			}
			log.Println("[myCache] Failed to get from peer", err)
		}
	}

	return g.getLocally(key)*/
}

// 从本地的回调函数获得key对应的值。
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err

	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

// 添加数据到缓存器
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}

// 利用【数据获得器】peer，从远程节点获得key对应的值。
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req:=&pb.Request{
		Group: g.name,
		Key: key,
	}
	res:=&pb.Response{}
	err := peer.Get(req,res)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
}


/*
func NewGroup(name0 string ,maxBytes int64,get0 Getter) *Group {
	if get0==nil{
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	ans:=&Group{
		name:name0,
		getter:get0,
		mainCache:cache{cacheBytes:maxBytes},
	}
	groups[name0]=ans
	return ans
}
*/