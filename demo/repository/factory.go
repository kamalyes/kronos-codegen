package repository

// RepositoryFactory 仓库工厂
// 清单往这个结构体加字段、往下面的构造函数加初始化项，两处靠约定名定位
//
// [EN] The repository factory; the manifest adds a field here and an initializer in the
// constructor below, both located by convention names
type RepositoryFactory struct {
	Seed *SeedRepository
}

// NewRepositoryFactory 创建仓库工厂
// [EN] NewRepositoryFactory builds the factory
func NewRepositoryFactory() *RepositoryFactory {
	return &RepositoryFactory{
		Seed: NewSeedRepository(),
	}
}
