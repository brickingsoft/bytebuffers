# bytebuffers

字节缓冲池。



## 特性
* 内存复用
* 可借出（必须归还）

## 场景
在网络等流中读数据并解析时，可以通过借出来减少一次内存分配和复制。

## 安装
```shell
go get -u github.com/brickingsoft/bytebuffers
```

## 使用
```go
buf := bytebuffers.Acquire()
defer bytebuffers.Release(buf)

rb := make([]byte, 4096)
wb := make([]byte, 4096)

rand.Read(wb)

buf.Write(wb)
buf.Read(rb)
```

## 性能

| 类型             | 速率          |
|----------------|-------------|
| bytebuffers    | 82.67 ns/op |
| bytes          | 98.56 ns/op |
