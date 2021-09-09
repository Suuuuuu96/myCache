# myCache
这是一个Golang分布式缓存框架。本框架的实现，参考了groupcache、memcached和GeeCache的开发思路和源代码。在此，我需要先向这些库的开发人员和相关贡献者表达我的敬意。
## 前言
> There are only two hard things in Computer Science: cache invalidation and naming things (计算科学中最难的两件事是命名和缓存失效)
> <p align="right">– Phil Karlton</p>


ORM 使用对象，封装了数据库操作，可以减少SQL语局的使用。开发者只使用面向对象编程，与数据对象直接交互，不用关心底层数据库。但是，ORM并不能完全取代SQL语句的使用。因为，对于复杂的查询，ORM 要么是无法表达，要么是性能不如原生的 SQL。
Golang并没有自带的ORM框架，比较流行的第三方框架有XORM、GORM等等。参考了XORM、GORM和GeeORM等框架，笔者也开发了一个简单的ORM框架——myORM，目前已具备了常见ORM需要的基础功能。
## 功能
* 对象与表框架的映射：本框架能基于传入对象解析其结构体（struct），在数据库中建立同名的数据表，根据结构体的字段设置同名、对应类型的数据表字段（属性）。
* 记录的插入：本框架能根据传入对象（或对象的切片）在数据表中插入一条（或多条）对应的记录。
* 记录的删除：本框架能接收参数并根据参数设置的条件删除数据表中符合条件的所有记录。
* 记录的修改：本框架能接收参数并根据参数设置的条件更新数据表中符合条件的所有记录。
* 记录的查询：本框架能能查询数据表中符合条件的所有记录并将其追加到指定的结构体切片。
* 钩子：本框架支持用户自定义八种钩子函数，分别位于增删改查四种操作的之前或之后。
* 迁移：结构体成员变更时，对应同名数据库表的字段将自动修改、更新。
* 事务：用户能自定义一系列操作，并将这些操作聚合成一个事务，该事务具备 ACID 四个属性。
## 框架重要概念
* Engine/引擎：用于连接数据库，一个引擎对应一个数据库。
* Session/会话：用于操作数据表（包括建立/删除表格、执行SQL语句、建立事务），一个会话对应一个数据表。一个引擎可以对应多个会话。
* Dialect/方言：不同的关系型数据库管理系统，使用的SQL语句可能有所不同。数据库的数据类型和Golang的数据类型也有差异（Golang的Int、Int8、Int16、Int32对应数据库的integer）。这些所有的差异均由Dialect来处理，之后各种操作均不需要考虑具体语言或数据的差异。一种数据库管理系统，对应一个方言。
* Field/字段：包含列名、类型和注解。一个字段对应数据库中的一个属性（一列），
* Schema/表框架：即数据表的组织和结构，包含程序中的对应模型、表名、各字段信息、全体列名。一个数据表对应一个表框架。
* generator/生成器：生成器负责生成SQL的各部分（如"WHERE ..."或"LIMIT ..."）。
* Clause/分句：一个Clause就是一条SQL语句的各部分的集合。
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
