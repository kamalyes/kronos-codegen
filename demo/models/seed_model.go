package models

// SeedModel 一个预先存在的实体，用来给各个锚点提供"已有成员"
// [EN] A pre-existing entity giving each anchor an existing member to attach to
type SeedModel struct {
	SeedID string
}

// TableName 返回表名
// [EN] TableName returns the table name
func (SeedModel) TableName() string { return "seeds" }
