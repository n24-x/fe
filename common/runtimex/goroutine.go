// Package runtimex 收纳 runtime 标准库缺失的少量辅助函数。
package runtimex

import "runtime"

// GoroutineID 返回当前 goroutine 的 ID。
//
// Go 标准库不公开该 API，故此处通过 runtime.Stack 解析其输出首行
// （形如 "goroutine 123 [running]:\n..."）取得。runtime.Stack 需做一次栈快照，
// 实测成本约数百 ns，**不得用于热路径**。
//
// 典型用途：识别"当前代码是否正跑在某个自建事件循环的 goroutine 上"，
// 以便该循环内的重入操作（重入发布 / 关闭自身）不阻塞自己造成自等死锁。
func GoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id uint64
	i := len("goroutine ")
	for ; i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
