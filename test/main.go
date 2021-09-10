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

/*

go run test/main.go -port=8003 -port=8002 -port=8001 -api=1
//fmt.Println([]byte(100))
	var port int
	var api bool
	// flag包实现了命令行参数的解析。
	// flag.Type(flag名, 默认值, 帮助信息)*Type
	// flag.TypeVar(Type指针, flag名, 默认值, 帮助信息)
	// 定义好命令行flag参数后，需要通过调用flag.Parse()来对命令行参数进行解析。
	flag.IntVar(&port, "port", 8001, "mycache server port")
	flag.BoolVar(&api, "api", false, "Start a api server?")
	flag.Parse()

	if api {
		go startAPIServer(apiAddr, g)
	}
	startCacheServer(addrMap[port], []string(addrs), g)
 */