package migrations

import "demo.local/svc/models"

// GetAllModels 返回所有模型，清单往这个切片追加元素
// [EN] GetAllModels returns every model; the manifest appends to this slice
func GetAllModels() []interface{} {
	return []interface{}{
		&models.SeedModel{},
	}
}
