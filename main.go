/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-01 16:20:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-01 20:10:35
 * @FilePath: \kronos-codegen\main.go
 * @Description: kronos-codegen 启动器
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package main

import "os"

// main 把标准流交给 Run，退出码由 Run 决定
// [EN] main hands the standard streams to Run, which decides the exit code
func main() {
	os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}
