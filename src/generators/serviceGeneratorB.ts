import { GeneratorConfig, FieldConfig } from '../types';
import { camelToSnake, isBusinessField, isTimestampField, generateFileHeader } from './index';

export function generateServiceStyleB(config: GeneratorConfig): string {
  const { entityName, entityNameLower, pbPackageAlias, pbType, primaryKeyField, fields, serviceName } = config;
  const hasStatus = fields.some(f => f.goName === 'Status' || (f.isEnum && f.goName.includes('Status')));

  const header = generateFileHeader(config, '业务逻辑层（独立结构体风格）');

  const createFields = fields.filter(f => !f.isCreatedAt && !f.isUpdatedAt && !f.isPrimaryKey);
  const updateFields = fields.filter(f => !f.isPrimaryKey && !f.isCreatedAt && !f.isUpdatedAt);

  const createModelLines = createFields.map(f => {
    const reqField = f.goName;
    return `\t\t${f.goName}: req.${reqField},`;
  }).join('\n');

  const updateModelLines = updateFields.map(f => {
    const reqField = f.goName;
    if (f.goType === 'string') {
      return `\texisting.${f.goName} = mathx.IfEmpty(req.${reqField}, existing.${f.goName})`;
    }
    if (f.isEnum) {
      return `\texisting.${f.goName} = mathx.IfEmpty(req.${reqField}, existing.${f.goName})`;
    }
    if (f.goType === 'int32' || f.goType === 'int64') {
      return `\texisting.${f.goName} = mathx.IfGtZero(req.${reqField}, existing.${f.goName})`;
    }
    return `\texisting.${f.goName} = req.${reqField}`;
  }).join('\n');

  const modelsPkg = config.modelsPackageName || 'models';
  const modelsImport = config.modelsImportPath || `"github.com/${config.moduleName}/models"`;
  const repoImport = config.repositoryImportPath || `"github.com/${config.moduleName}/repository"`;
  const errorsImport = config.errorsImportPath || `"github.com/${config.moduleName}/errors"`;
  const servicePkg = config.servicePackageName || 'service';

  return `${header}
package ${servicePkg}

import (
\t"context"

\t${errorsImport}
\t${modelsImport}
\t${repoImport}
\tpb "${config.pbPackage}"
\t"github.com/kamalyes/go-pbmo"
\tsqlbuilderRepo "github.com/kamalyes/go-sqlbuilder/repository"
\t"gorm.io/gorm"
)

type ${entityName}Service struct {
\t${pbPackageAlias}.Unimplemented${serviceName}Server
\trepo           *repository.RepositoryFactory
\tconverter      *pbmo.BidiConverter
}

func New${entityName}Service() *${entityName}Service {
\tconverter := pbmo.NewBidiConverter(
\t\t&${pbPackageAlias}.${pbType}{},
\t\t&${modelsPkg}.${entityName}Model{},
\t)

\treturn &${entityName}Service{
\t\trepo:      repository.NewRepositoryFactory(),
\t\tconverter: converter,
\t}
}

func (s *${entityName}Service) modelToProto(model *${modelsPkg}.${entityName}Model) (*${pbPackageAlias}.${pbType}, error) {
\tpbMsg := &${pbPackageAlias}.${pbType}{}
\tif err := s.converter.ConvertModelToPB(model, pbMsg); err != nil {
\t\treturn nil, err
\t}
\treturn pbMsg, nil
}

func (s *${entityName}Service) ${entityName}Create(ctx context.Context, req *${pbPackageAlias}.${entityName}CreateRequest) (*${pbPackageAlias}.${entityName}CreateResponse, error) {
\tm := &${modelsPkg}.${entityName}Model{
${createModelLines}
\t}

\tcreated, err := s.repo.${entityName}.Create(ctx, m)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}CreateFailed)
\t}

\tpbMsg, err := s.modelToProto(created)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeConversionFailed)
\t}

\treturn Ok[${entityName}CreateResp](pbMsg), nil
}

func (s *${entityName}Service) ${entityName}Get(ctx context.Context, req *${pbPackageAlias}.${entityName}GetRequest) (*${pbPackageAlias}.${entityName}GetResponse, error) {
\tm, err := s.repo.${entityName}.GetByID(ctx, req.${primaryKeyField})
\tif err != nil {
\t\tif err == gorm.ErrRecordNotFound {
\t\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}NotFound)
\t\t}
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeDatabaseOperationFailed)
\t}

\tpbMsg, err := s.modelToProto(m)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeConversionFailed)
\t}

\treturn Ok[${entityName}GetResp](pbMsg), nil
}

func (s *${entityName}Service) ${entityName}List(ctx context.Context, req *${pbPackageAlias}.${entityName}ListRequest) (*${pbPackageAlias}.${entityName}ListResponse, error) {
\tquery := sqlbuilderRepo.NewQuery()
${buildListQueryFiltersB(config)}

\tlist, err := s.repo.${entityName}.List(ctx, query)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeDatabaseOperationFailed)
\t}

\titems := make([]*${pbPackageAlias}.${pbType}, 0, len(list))
\tfor _, m := range list {
\t\tpbMsg, convErr := s.modelToProto(m)
\t\tif convErr != nil {
\t\t\tcontinue
\t\t}
\t\titems = append(items, pbMsg)
\t}

\treturn &${pbPackageAlias}.${entityName}ListResponse{
\t\t${pbType}s: items,
\t\tTotal:      int64(len(items)),
\t}, nil
}

func (s *${entityName}Service) ${entityName}Update(ctx context.Context, req *${pbPackageAlias}.${entityName}UpdateRequest) (*${pbPackageAlias}.${entityName}UpdateResponse, error) {
\texisting, err := s.repo.${entityName}.GetByID(ctx, req.${primaryKeyField})
\tif err != nil {
\t\tif err == gorm.ErrRecordNotFound {
\t\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}NotFound)
\t\t}
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeDatabaseOperationFailed)
\t}

${updateModelLines}

\tupdated, err := s.repo.${entityName}.Update(ctx, existing)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}UpdateFailed)
\t}

\tpbMsg, err := s.modelToProto(updated)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeConversionFailed)
\t}

\treturn Ok[${entityName}UpdateResp](pbMsg), nil
}

func (s *${entityName}Service) ${entityName}Delete(ctx context.Context, req *${pbPackageAlias}.${entityName}DeleteRequest) (*${pbPackageAlias}.${entityName}DeleteResponse, error) {
\tif err := s.repo.${entityName}.DeleteByID(ctx, req.${primaryKeyField}); err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}DeleteFailed)
\t}
\treturn Ok[${entityName}DeleteResp](true), nil
}
${hasStatus ? `
func (s *${entityName}Service) ${entityName}UpdateStatus(ctx context.Context, req *${pbPackageAlias}.${entityName}UpdateStatusRequest) (*${pbPackageAlias}.${entityName}UpdateStatusResponse, error) {
\tif err := s.repo.${entityName}.UpdateStatus(ctx, req.${primaryKeyField}, int32(req.Status)); err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}UpdateStatusFailed)
\t}
\treturn &${pbPackageAlias}.${entityName}UpdateStatusResponse{}, nil
}` : ''}
`;
}

function buildListQueryFiltersB(config: GeneratorConfig): string {
  const filterFields = config.fields.filter(f =>
    !f.isPrimaryKey && !f.isCreatedAt && !f.isUpdatedAt &&
    (f.goType === 'string' || f.isEnum)
  );

  if (filterFields.length === 0) {
    return '\t// TODO: 添加过滤条件';
  }

  return filterFields.map(f => {
    const snakeName = camelToSnake(f.goName);
    if (f.isEnum) {
      return `\tquery.AddFilterIfNotEmpty("${snakeName}", req.${f.goName})`;
    }
    return `\tquery.AddFilterIfNotEmpty("${snakeName}", req.${f.goName})`;
  }).join('\n');
}
