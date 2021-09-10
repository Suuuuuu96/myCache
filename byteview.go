package mycache

//b 将会存储真实的缓存值。选择 byte 类型是为了能够支持任意的数据类型的存储，
//例如字符串、图片等。
type ByteView struct {
	b []byte
}

func (v ByteView)Len() int64 {
	return int64(len(v.b))
}

func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}

func (v ByteView)String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
/*
func (v *ByteView)Copy() []byte {
	c:=make([]byte,len(v.b))
	copy(c,v.b)
	return c
}*/