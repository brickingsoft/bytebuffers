# bytebuffers

字节缓冲池。

## 安装
```shell
go get -u github.com/brickingsoft/bytebuffers
```


## 特性
* 预分配
* 可借出（必须归还）
* 在`linux`下使用`mmap`进行分配

## 案例
在网络等流中读数据并解析时，可以通过借出来减少一次内存分配和复制。
