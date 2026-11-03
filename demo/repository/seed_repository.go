package repository

// SeedRepository 预先存在的仓库
// [EN] A pre-existing repository
type SeedRepository struct{}

// NewSeedRepository 创建仓库
// [EN] NewSeedRepository builds the repository
func NewSeedRepository() *SeedRepository { return &SeedRepository{} }
