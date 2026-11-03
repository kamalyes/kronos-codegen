package seed

// SeedServiceImpl 预先存在的领域服务
// [EN] A pre-existing domain service
type SeedServiceImpl struct{}

// NewSeedService 创建领域服务
// [EN] NewSeedService builds the domain service
func NewSeedService() *SeedServiceImpl { return &SeedServiceImpl{} }
