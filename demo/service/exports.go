package service

import (
	seedsvc "demo.local/svc/service/seed"
)

// 新增实体的服务别名插在这里 kronos:exports
type (
	SeedServiceImpl = seedsvc.SeedServiceImpl
)

// 新增实体的构造别名插在这里 kronos:exports
var (
	NewSeedService = seedsvc.NewSeedService
)
