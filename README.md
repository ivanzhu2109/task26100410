# kitchen

厨房餐票进度归并库（纯进程内存，并发安全），供上层直接调用。

## 用法

```go
s := kitchen.NewStore(map[string]string{"SKU-1": "grill"}) // SKU→工位映射，初始化后不变

s.ApplyRevision(kitchen.Revision{TicketID: "T1", Version: 1, Lines: []kitchen.Line{
    {LineID: "L1", SKU: "SKU-1", Qty: 5},
}}) // 只接受更高版本；减量按"先撤未完工、再撤已完工，均从最近新增撤起"

view := s.View("T1")        // 分工位的各行完成/待制作数量 + 每工位一个完工凭据 + 整票状态
s.Submit(view.Stations[0].Credential) // 凭据只覆盖查看时该工位未完成的份，可重复/乱序提交
```

## 测试

```sh
go test ./...
```
