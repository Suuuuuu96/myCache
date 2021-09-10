package mycache

import (
	"fmt"
	"io/ioutil"
	"log"
	"mycache/consistenthash"
	pb "mycache/mycachepb"
	"github.com/golang/protobuf/proto"
	"net/http"
	"net/url"
	"strings"
	"sync"
)
const (
	defaultBasePath = "/_mycache/"
	defaultReplicas = 50
)


// HTTPPool implements PeerPicker for a pool of HTTP peers.
// HTTPPool 只有 2 个参数，一个是 self，用来记录自己的地址，包括主机名/IP 和端口。
// 另一个是 basePath，作为节点间通讯地址的前缀，默认是 /_mycache/，
// 那么 http://example.com/_mycache/ 开头的请求，就用于节点间的访问。
// 因为一个主机上还可能承载其他的服务，加一段 Path 是一个好习惯。
// 比如，大部分网站的 API 接口，一般以 /api 作为前缀。
type HTTPPool struct {
	// 字段：
	// this peer's base URL, e.g. "https://example.net:8000"
	self        string
	basePath    string
	mu          sync.Mutex // guards peers and httpGetters
	peers       *consistenthash.Map // 一致性哈希算法的字典，用来根据具体的 key 选择节点。peers是键值指向IP地址，如"小明"→"http://10.0.0.2:8008"。
	httpGetters map[string]*httpGetter // keyed by e.g. "http://10.0.0.2:8008"。httpGetters是一个IP地址指向一个【数据获得器】。
	// 即每一个远程节点（的IP地址）指向一个 httpGetter。httpGetter 与远程节点的地址 baseURL 有关。

	// 方法：
	// ServeHTTP(w http.ResponseWriter, r *http.Request) ：响应其他节点的请求。
	// Set(peers ...string) ：传入所有节点（包括本节点）的IP地址的集合，设置同辈节点的信息。
	// PickPeer(key string) (PeerGetter, bool)： 返回键值对应的【数据获得器】。
}

// NewHTTPPool initializes an HTTP pool of peers.
func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self, // 如"localhost:9999"
		basePath: defaultBasePath,
	}
}

// Log info with server name
func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}


// 服务器部分：
// ServeHTTP handle all http requests
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// strings.Hasprefix(s, prefix)返回s是否以prefix开头
	if !strings.HasPrefix(r.URL.Path, p.basePath) { //如果r.URL.Path不以"/_mycache/"开头
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	p.Log("%s %s", r.Method, r.URL.Path)

	// p.basePath是/_mycache/
	// r.URL.Path是/_mycache/scores/Tom
	// r.URL.Path[len(p.basePath):]是scores/Tom
	// /<basepath>/<groupname>/<key> required
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	// parts[0]是scores，parts[1]是Tom
	if len(parts) != 2 {
		//如果parts不是group+key，则非法
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}
	view, err := group.Get(key)
	body, err := proto.Marshal(&pb.Response{Value: view.ByteSlice()})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body) //按事先协商好：body必须是序列化的pb.Response格式的数据。

	//if err != nil {
	//	http.Error(w, err.Error(), http.StatusInternalServerError)
	//	return
	//}
	//
	//w.Header().Set("Content-Type", "application/octet-stream")
	//w.Write(view.ByteSlice())

}
/*
func SplitN(s, sep string, n int) []string
在这里，s是字符串，sep是分隔符。如果s不包含给定的sep且sep为非空，则它将返回长度为1的切片，其中仅包含s。
或者，如果sep为空，则它将在每个UTF-8序列之后拆分。或者，如果s和sep均为空，则它将返回一个空切片。
在这里，最后一个参数确定函数要返回的字符串数。可以是以下任何一种：n等于零(n == 0)：结果为nil，即零个子字符串。返回一个空列表。
n大于零(n> 0)：最多返回n个子字符串，最后一个字符串为未分割的余数。n小于零(n <0)：将返回所有可能的子字符串。
strings.SplitN("a:b:c:d:e:f", ":", 3) //[a b c:d:e:f]
*/


// 客户端部分：


// Set updates the pool's list of peers.
// peers是所有节点（包括本节点）的IP地址的集合
func (p *HTTPPool) Set(peerIPs ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil) // 新建一个一致性哈希字典。
	p.peers.Add(peerIPs...)
	p.httpGetters = make(map[string]*httpGetter, len(peerIPs))
	for _, peer := range peerIPs {
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
		// 一个IP地址，指向一个数据获得器。
	}
}

// PickPeer picks a peer according to key
// 先通过p.peers的一致性哈希获得键值key对应的节点的IP地址，然后返回该IP地址对应的数据获得器httpGetters。
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		// peer是按一致性哈希字典得到的IP地址，如"https://example.net:8000"
		p.Log("Pick peer %s", peer)
		return p.httpGetters[peer], true
		//如果peer不是空且不是本节点，则返回peer对应的【数据获得器】。
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)
//检查HTTPPool是否实现了【数据获得器的选择器】PeerPicker接口





// 创建 httpGetter，实现 PeerGetter 接口。——基于HTTP的【数据获得器】。
type httpGetter struct {
	baseURL string //表示将要访问的远程节点的地址，例如 http://example.com/_mycache/。
}

func (h *httpGetter) Get(in *pb.Request, out *pb.Response) error {
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(in.GetGroup()),
		url.QueryEscape(in.GetKey()),
		//func QueryEscape(s string) string ：该函数对s进行转码使之可以安全的用在URL查询里。
		//url.QueryEscape("http://images.com /cat.png")的结果是"http%3A%2F%2Fimages.com+%2Fcat.png"
	)
	res, err := http.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}
	//需要事先协商好：传来的数据必须是序列化的pb.Response格式的数据。
	bytes, err := ioutil.ReadAll(res.Body) //ReadAll() 是一次读取所有数据
	if err = proto.Unmarshal(bytes, out); err != nil {
		return fmt.Errorf("decoding response body: %v", err)
	}
	/*if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}*/

	return nil
}
var _ PeerGetter = (*httpGetter)(nil)



/*
func (p *HTTPPool)ServeHTTP2(w http.ResponseWriter,r *http.Request)  {
	if !strings.HasPrefix(r.URL.Path,p.basePath){
		http.Error(w,"error1",http.StatusInternalServerError)
	}
	data:=strings.SplitN(r.URL.Path[len(p.basePath):],"/",2)
	if len(data)!=2{
		http.Error(w,"error1",http.StatusInternalServerError)
	}
	g:=GetGroup(data[0])
	if g==nil{
		http.Error(w,"error1",http.StatusInternalServerError)
	}
	temp,err:=g.Get(data[1])
	if err!=nil{
		http.Error(w,"error1",http.StatusInternalServerError)
	}
	w.Write(temp.b)
}*/