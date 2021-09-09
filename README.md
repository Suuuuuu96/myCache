# myCache
这是一个Golang分布式缓存框架。本框架的实现，参考了groupcache、memcached和GeeCache的开发思路和源代码。在此，我需要先向这些库的开发人员和相关贡献者表达我的敬意。
## 前言
> There are only two hard things in Computer Science: cache invalidation and naming things (计算科学中只有两件难事：缓存失效和命名。)
> <p align="right">– Phil Karlton</p>
在高并发的分布式的系统中，缓存是必不可少的一部分。没有缓存对系统的加速和阻挡大量的请求直接落到系统的底层，系统是很难撑住高并发的冲击，所以分布式系统中缓存的设计是很重要的一环。Golang并没有自带的分布式缓存框架，比较流行的第三方框架有groupcache等。参考了相关框架的开发思路和源代码后，笔者开发了一个简单的分布式缓存框架——myCache，目前已具备了常见分布式缓存框架需要的基础功能。
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
* HTTPPool：所有节点之间的HTTP通讯任务，均由HTTPPool来负责。ServeHTTP()负责响应其他节点的请求，httpGetters负责向其他节点发出请求。
* consistent hash/一致性哈希：consistenthash.Map是基于一致性哈希的字典。功能：对于给定的key值，返回对应缓存节点（的IP地址）；或者添加节点。
## 样例程序
```
package main
import (
	"fmt"
	"myorm"
	"myorm/session"
)
//注意结构体里面，首字母大写才能被识别到
type USER struct {
	Name string `myorm:"PRIMARY KEY"`
	Age  int
	School  string
}
func (u *USER)BeforeInsert(s *session.Session) error { //钩子函数：插入前
	fmt.Println("调用BeforeInsert函数：即将插入",*u,"。")
	return nil
}
func (u *USER)AfterInsert(s *session.Session) error { //钩子函数：插入后
	fmt.Println("调用AfterInsert函数：插入即将完成。")
	return nil
}
func main() {

	// 1、声明并赋值几个实例
	var (
		user1 = &USER{"ou", 18,"CAU"}
		user2 = &USER{"Sam", 25,"GU"}
		user3 = &USER{"Amy", 21,"HU"}
		user4 = &USER{"su", 21,"HU"}
	)

	// 2、新建一个连接数据库的引擎，数据库文件是同一目录下的newDB.db
	en,_:=myorm.NewEngine("sqlite3","newDB.db")

	// 3、新建一个会话，并根据一个空的USER对象创建表框架
	s := en.NewSession().Model(&USER{})

	// 4、如果已经存在USER表，则先删除
	_ = s.DropTable()

	// 5、建立USER表
	_ = s.CreateTable()

	// 6、向USER表插入2个实例
	_, _ = s.Insert(user1, user2)

	// 7、查询一共有几条记录
	num,_ := s.Count()
	fmt.Println("事务前记录个数：",num) //输出："事务前记录个数： 2"

	// 8、新建一个事务（向USER表插入2个实例）
	s,_,_ =s.Transaction(func(s0 *session.Session) (*session.Session,interface{}, error) {
		_, _ = s0.Insert(user3,user4)
		return s0, nil, nil
		//return nil, errors.New("err")
	})
	num,_=s.Count()
	fmt.Println("事务后记录个数：",num)  //输出："事务后记录个数： 4"

	// 9、将Amy的年龄修改为30
	_, _ = s.Where("Name = ?", "Amy").Update("Age", 30)

	// 10、删除学校为"CAU"的学生并输出成功删除记录数
	deleteNum,_:=s.Where("School=?","CAU").Delete()
	fmt.Println("已删除",deleteNum,"条记录")  //输出："事务后记录个数： 4"

	// 11、将名为"Amy"的记录追加到[]USER切片，且最多不超过三条记录（实际上只有一条，因为Name是主键）
	var users []USER
	_ = s.Where("Name = ?", "Amy").Limit(3).Find(&users)
	fmt.Println(users) //输出："[{Amy 30 HU}]"

	// 12、将年纪大于等于18的记录追加到[]USER切片，且最多不超过2条记录（符合条件的其实有三条）
	_ = s.Where("Age>=?",18).Limit(2).Find(&users)
	fmt.Println(users) //输出："[{Amy 30 HU} {Sam 25 GU} {Amy 30 HU}]"

	// 13、查询年纪小于22的有几条记录（链式操作）
	num2,_:=s.Where("Age<?",22).Count()

	// 14、查询年纪大于等于18的有几条记录
	s.Where("Age>=?",18)
	num3,_:=s.Count()

	// 15、查看查询结果
	fmt.Println(num2,num3) //输出："1 3"

}
```

### 注意事项
#### 函数执行顺序
直接执行Clear()的函数：Exec()、QueryRows()、QueryRow()<br>
间接执行Clear()的函数：Insert()、Find()、Update()、Delete()、Count()、First()<br>
不会执行Clear()的函数：Limit()、Where()、OrderBy()<br>
执行Clear()后，会话的SQL语句及其参数都会被清空。<br>
链式操作时，须注意使不会执行Clear()的函数在前面，其他在后面。
#### 事务的执行
数据库事务(transaction)是访问并可能操作各种数据项的一个数据库操作序列，这些操作要么全部执行,要么全部不执行，是一个不可分割的工作单位。<br>
事务由事务开始与事务结束之间执行的全部数据库操作组成。分三个阶段：开始；读写；提交或回滚。
事务开始之后不断进行读写操作，但写操作仅仅将数据写入磁盘缓冲区，而非真正写入磁盘内。顺利完成所有操作则提交，数据保存到磁盘；否则回滚。<br>
本框架中事务的实现有两种，分别是Session的method和Engine的的method。<br>

#### 钩子函数
Hook 的意思是钩住，也就是在消息过去之前，先把消息钩住，不让其传递，使用户可以优先处理。
执行这种操作的函数也称为钩子函数。<br>
“先钩住再处理”，执行某操作之前，优先处理一下，再决定后面的执行走向。<br>
本框架可以让用户自定义8个钩子函数，分别在增删改查操作的前后发生。<br>
例子：
```
type Account struct {
	ID       int `myorm:"PRIMARY KEY"`
	Password string
}
func (account *Account) BeforeInsert(s *Session) error {
	log.Info("before inert", account)
	account.ID += 1000
	return nil
}
func (account *Account) AfterQuery(s *Session) error {
	log.Info("after query", account)
	account.Password = "******"
	return nil
}
```

声明一个账号类，类有一个BeforeInsert函数，则在每次插入记录时均会调用该函数。<br>
有一个AfterQuery函数，则在每次查询前均会调用该函数。<br>
说明：<br>
这些钩子函数必须是结构体（如Account）的对应method。<br>
且函数格式必须是：<br>
```
func (指针名 *类名) 函数名(s *Session) error {
	操作
	return nil
}
```
其中，函数名必须是"BeforeQuery"、"AfterQuery"、"BeforeUpdate"、"AfterUpdate"、"BeforeDelete"、"AfterDelete"、"BeforeInsert"、"AfterInsert"之一。<br>
这些函数接收且仅接收一个参数：对应的会话的指针。<br>
用户可以根据自己的需要，利用获得的对应会话的指针，设置自己需要的操作（当然也可以不使用该会话）。用户只能操作 Account指针 或 Session指针 ，不能返回值。函数仅返回一个error。
