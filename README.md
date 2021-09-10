# myCache
这是一个Golang分布式缓存框架。本框架的实现，参考了groupcache、memcached和GeeCache的开发思路和源代码。在此，我需要先向这些库的开发人员和相关贡献者表达我的敬意。
## 前言
> There are only two hard things in Computer Science: cache invalidation and naming things (计算科学中只有两件难事：缓存失效和命名。)
> <p align="right">– Phil Karlton</p>
在高并发的分布式的系统中，缓存是必不可少的一部分。<br>没有缓存对系统的加速和阻挡大量的请求直接落到系统的底层，系统是很难撑住高并发的冲击，所以分布式系统中缓存的设计是很重要的一环。Golang并没有自带的分布式缓存框架，比较流行的第三方框架有groupcache等。参考了相关框架的开发思路和源代码后，笔者开发了一个简单的分布式缓存框架——myCache，目前已具备了常见分布式缓存框架需要的基础功能。
<br>本框架中，键值需要是string类型，缓存值需要是[]byte类型（或可以转成为[]byte）。选择 byte 类型是为了能够支持任意的数据类型的存储，例如字符串、图片等。
## 功能
* 缓存的分布式存储：本框架利用一致性哈希(consistent hashing)算法确定各键值的对应缓存节点（的IP地址），同时引入虚拟节点解决数据倾斜问题。
* 节点通讯：本框架中，每一个节点都同时是基于HTTP的服务端和客户端，能向其他节点发出请求、也能响应其他节点的请求。
* 缓存淘汰：本框架实现了LRU(Least Recently Used，最近最少使用)算法，及时淘汰不常用缓存数据，保证了一定容量下缓存的正常使用。
* 单机并发：本框架利用Go语言自带的互斥锁，保证了单机并发读写时的数据安全。
* Single Flight：本框架使用Single Flight机制，合并较短时间内相继达到的针对同一键值的请求，抑制重复的函数调用，防止缓存击穿。
* Protobuf 通信：本框架使用Protocol Buffers作为数据描述语言，显著地压缩了二进制数据传输的大小，降低了节点之间通讯的成本。
## 框架重要概念
* Group/组：同一个类型或领域的内容，由同一个Group来存储。一个节点可以存储多个Group的（部分）数据，一个Group的内容可以分布在多个节点。
* 节点：在本框架中，一个可以存储数据并使用HTTP通讯的主机就是一个节点。
* single flight：直译为单程飞行。短时间内，最早到来的请求将调用获得数据的函数。而其他请求不再调用该函数，而是等待着分享最早请求获得的数据。
* HTTPPool：所有节点之间的HTTP通讯任务，均由HTTPPool来负责。HTTPPool的方法ServeHTTP()负责响应其他节点的请求，HTTPPool的字段httpGetters负责向其他节点发出请求。
* consistent hash/一致性哈希：consistenthash.Map是基于一致性哈希的字典。功能：对于给定的key值，返回对应缓存节点（的IP地址）；或者添加节点。
* PeerGetter：【数据获得器】接口，实现该接口的结构体必须：能从指定group获得指定key对应的值，并返回。
* PeerPicker：【数据获得器的选择器】接口，实现该接口的结构体必须能根据传入的 key 找到并返回对应的PeerGetter【数据获得器】。
## 样例程序
```
package main

import (
	"fmt"
	"log"
	"mycache"
	"net/http"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}
func createGroup() *mycache.Group {
	return mycache.NewGroup("scores", 2<<10, mycache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}
func startCacheServer(addr string, addrs []string, g *mycache.Group) {
	peers := mycache.NewHTTPPool(addr)
	peers.Set(addrs...)
	g.RegisterPeers(peers)
	log.Println("mycache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}
func startAPIServer(apiAddr string, g *mycache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := g.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())

		}))
	log.Println("fontend server is running at", apiAddr)

	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
	//log.Fatal里面的内容出错则：打印输出内容；退出应用程序；defer函数不会执行。

}

func main() {
	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}
	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}
	g := createGroup()
	g2 := createGroup()
	g3 := createGroup()
	g4 := createGroup()

	go startCacheServer(addrMap[8001], []string(addrs), g)
	go startCacheServer(addrMap[8002], []string(addrs), g2)
	go startCacheServer(addrMap[8003], []string(addrs), g3)
	startAPIServer(apiAddr, g4)

	/*
	测试
	http://localhost:9999/api?key=Tom

	http://localhost:8001/_mycache/scores/Tom
	*/
}

```

### 说明&注意事项
#### single flight
常见的缓存使用问题包括：<br>
缓存雪崩：缓存在同一时刻全部失效，造成瞬时DB请求量大、压力骤增，引起雪崩。缓存雪崩通常因为缓存服务器宕机、缓存的 key 设置了相同的过期时间等引起。（多个key）<br>
缓存击穿：一个存在的key，在缓存过期的一刻，同时有大量的请求，这些请求都会击穿到 DB ，造成瞬时DB请求量大、压力骤增。（一个存在的key）<br>
缓存穿透：查询一个不存在的数据，因为不存在则不会写到缓存中，所以每次都会去请求 DB，如果瞬间流量过大，穿透到 DB，导致宕机。（一个不存在的key）<br>
single flight机制可以解决缓存击穿问题。

#### HTTPPool
HTTPPool implements PeerPicker for a pool of HTTP peers.HTTPPool 只有 2 个参数，一个是 self，用来记录自己的地址，包括主机名/IP 和端口。
另一个是 basePath，作为节点间通讯地址的前缀，默认是 /mycache/，那么 http://example.com/mycache/ 开头的请求，就用于节点间的访问。
因为一个主机上还可能承载其他的服务，加一段 Path 是一个好习惯。比如，大部分网站的 API 接口，一般以 /api 作为前缀。<br>
HTTPPool的字段：<br>
self        string<br>
basePath    string<br>
mu          sync.Mutex // guards peers and httpGetters<br>
peers       * consistenthash.Map // 一致性哈希算法的字典，用来根据具体的 key 选择节点。peers是键值指向IP地址，如"小明"→"http://10.0.0.2:8008"。
httpGetters map[string]* httpGetter <br>
//httpGetters是一个IP地址指向一个【数据获得器】。即每一个远程节点（的IP地址）指向一个 httpGetter。httpGetter 与远程节点的地址 baseURL 有关。<br>
HTTPPool的方法：<br>
ServeHTTP(w http.ResponseWriter, r * http.Request) ：响应其他节点的请求。<br>
Set(peers ...string) ：传入所有节点（包括本节点）的IP地址的集合，设置同辈节点的信息。<br>
PickPeer(key string) (PeerGetter, bool)： 返回键值对应的【数据获得器】。<br>

#### consistent hash
结构体consistenthash.Map是基于一致性哈希的字典。功能：对于给定的key值，返回对应缓存节点（的名称）；或者添加节点。<br>
hash：哈希算法，默认是crc32.ChecksumIEEE<br>
replicas：虚拟节点倍数。<br>
keys：哈希环。节点的名称对应的哈希值。一个真实节点对应多个虚拟节点。<br>
hashMap：虚拟节点与真实节点的映射表 hashMap，键是虚拟节点的哈希值，值是真实节点的名称。<br>
节点名称哈希值到节点名称的映射。由于虚拟节点的存在，可能有多个哈希值对应一个真实节点。<br>
每个真实节点有一个唯一的名称作为标识符。<br>
