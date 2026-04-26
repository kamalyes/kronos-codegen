import { GeneratorConfig } from '../types';
import { camelToSnake, isBusinessField, generateFileHeader } from './index';

export function generateRepository(config: GeneratorConfig): string {
  const { entityName, entityNameLower, primaryKeyField, tableName } = config;
  const repoName = `${entityName}Repository`;
  const pkSnake = camelToSnake(primaryKeyField);
  const pkMethodSuffix = primaryKeyField.replace(/_/g, '');

  const businessFields = config.fields.filter(isBusinessField);
  const hasTenant = config.fields.some(f => f.goName === 'TenantId');
  const hasCode = config.fields.some(f => f.goName === 'Code');
  const hasStatus = config.fields.some(f => f.goName === 'Status' || f.isEnum && f.goName.includes('Status'));

  const header = generateFileHeader(config, '数据访问层');
  const entityComment = config.tableComment || entityName;
  const modelsPkg = config.modelsPackageName || 'models';
  const modelsImport = config.modelsImportPath || `"github.com/${config.moduleName}/models"`;
  const repoPkg = config.repositoryPackageName || 'repository';

  let extraMethods = '';

  if (hasCode) {
    extraMethods += `
// CodeExists 检查${entityComment}编码是否已存在
func (r *${repoName}) CodeExists(ctx context.Context, ${pkSnake} string, code string) (bool, error) {
\tcount, err := r.Count(ctx,
\t\trepository.NewEqFilter("tenant_id", ${pkSnake}),
\t\trepository.NewEqFilter("code", code),
\t)
\tif err != nil {
\t\treturn false, err
\t}
\treturn count > 0, nil
}
`;
  }

  if (hasTenant) {
    extraMethods += `
// ListByTenantID 根据租户ID查询${entityComment}列表
func (r *${repoName}) ListByTenantID(ctx context.Context, tenantID string) ([]*${modelsPkg}.${entityName}Model, error) {
\treturn r.List(ctx, repository.NewQuery().AddEqual("tenant_id", tenantID))
}
`;
  }

  return `${header}
package ${repoPkg}

import (
\t"context"
\t${modelsImport}
\tgwglobal "github.com/kamalyes/go-rpc-gateway/global"
\t"github.com/kamalyes/go-sqlbuilder/db"
\t"github.com/kamalyes/go-sqlbuilder/repository"
)

// ${repoName} ${entityComment}仓库结构体，继承通用基础仓库
type ${repoName} struct {
\t*repository.BaseRepository[${modelsPkg}.${entityName}Model]
}

// New${repoName} 创建${entityComment}仓库实例
func New${repoName}() *${repoName} {
\tdbHandler := db.MustNewGormHandler(gwglobal.DB)
\treturn &${repoName}{
\t\tBaseRepository: repository.NewBaseRepository(
\t\t\tdbHandler,
\t\t\tgwglobal.GetLogger(),
\t\t\t${modelsPkg}.${entityName}Model{}.TableName(),
\t\t\trepository.WithAutoFields[${modelsPkg}.${entityName}Model](),
\t\t),
\t}
}

// GetByID 根据ID获取${entityComment}信息
func (r *${repoName}) GetByID(ctx context.Context, ${pkSnake} string) (*${modelsPkg}.${entityName}Model, error) {
\treturn r.GetByFilter(ctx, repository.NewEqFilter("${pkSnake}", ${pkSnake}))
}

// ListWithPagination 分页查询${entityComment}列表
func (r *${repoName}) ListWithPagination(ctx context.Context, query *repository.Query, pagination *repository.Pagination32) ([]*${modelsPkg}.${entityName}Model, *repository.Pagination32, error) {
\treturn r.ListWithPagination32(ctx, query, pagination)
}

// UpdateBy${pkMethodSuffix} 根据ID更新${entityComment}指定字段
func (r *${repoName}) UpdateBy${pkMethodSuffix}(ctx context.Context, ${pkSnake} string, updates map[string]interface{}) error {
\treturn r.UpdateFieldsByFilters(ctx, updates, repository.NewEqFilter("${pkSnake}", ${pkSnake}))
}

// DeleteByID 根据ID删除${entityComment}
func (r *${repoName}) DeleteByID(ctx context.Context, ${pkSnake} string) error {
\treturn r.DeleteByFilters(ctx, repository.NewEqFilter("${pkSnake}", ${pkSnake}))
}
${hasStatus ? `
// UpdateStatus 更新${entityComment}状态
func (r *${repoName}) UpdateStatus(ctx context.Context, ${pkSnake} string, status int32) error {
\treturn r.UpdateFieldsByFilters(ctx, map[string]interface{}{"status": status}, repository.NewEqFilter("${pkSnake}", ${pkSnake}))
}` : ''}${extraMethods}`;
}
