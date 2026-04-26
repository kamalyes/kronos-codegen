import { GeneratorConfig, FieldConfig } from '../types';
import { camelToSnake, isBusinessField, isTimestampField, isSkipField, generateFileHeader } from './index';

export function generateServiceStyleA(config: GeneratorConfig): string {
  const { entityName, entityNameLower, pbPackageAlias, pbType, primaryKeyField, fields } = config;
  const pkSnake = camelToSnake(primaryKeyField);
  const hasStatus = fields.some(f => f.goName === 'Status' || (f.isEnum && f.goName.includes('Status')));
  const hasTenant = fields.some(f => f.goName === 'TenantId');

  const header = generateFileHeader(config, '业务逻辑层（单结构体风格）');

  const createFields = fields.filter(f => !f.isCreatedAt && !f.isUpdatedAt && !f.isPrimaryKey);
  const updateFields = fields.filter(f => !f.isPrimaryKey && !f.isCreatedAt && !f.isUpdatedAt);

  const createModelFields = createFields.map(f => {
    const reqField = protoNameFromGoName(f.goName);
    if (f.isEnum) {
      return `\t\t${f.goName}: req.${reqField},`;
    }
    if (f.goType === 'string') {
      return `\t\t${f.goName}: req.${reqField},`;
    }
    if (f.goType === 'bool') {
      return `\t\t${f.goName}: req.${reqField},`;
    }
    if (f.goType === 'int32') {
      return `\t\t${f.goName}: req.${reqField},`;
    }
    return `\t\t${f.goName}: req.${reqField},`;
  }).join('\n');

  const updateBuilder = updateFields.map(f => {
    const reqField = protoNameFromGoName(f.goName);
    const snakeName = camelToSnake(f.goName);
    if (f.isEnum) {
      return `\t\tSetIfNotZero("${snakeName}", int32(req.${reqField}.GetValue()))`;
    }
    if (f.goType === 'string') {
      return `\t\tSetIfNotEmpty("${snakeName}", req.${reqField}.GetValue())`;
    }
    if (f.goType === 'bool') {
      return `\t\tSetIfNotDefault("${snakeName}", req.${reqField})`;
    }
    if (f.goType === 'int32' || f.goType === 'int64') {
      return `\t\tSetInt32Val("${snakeName}", req.${reqField})`;
    }
    if (isTimestampField(f)) {
      return '';
    }
    return `\t\tSetStringVal("${snakeName}", req.${reqField})`;
  }).filter(s => s !== '').join('\n');

  const listQueryFilters = buildListQueryFilters(config);
  const modelsPkg = config.modelsPackageName || 'models';
  const modelsImport = config.modelsImportPath || `"github.com/${config.moduleName}/models"`;
  const errorsImport = config.errorsImportPath || `"github.com/${config.moduleName}/errors"`;
  const servicePkg = config.servicePackageName || 'service';

  return `${header}
package ${servicePkg}

import (
\t"context"
\t${errorsImport}
\t${modelsImport}
\tpb "${config.pbPackage}"
\t"github.com/kamalyes/go-pbmo"
\t"github.com/kamalyes/go-sqlbuilder/repository"
)

func (s *ServiceImpl) ${entityName}Create(ctx context.Context, req *pb.${entityName}CreateRequest) (*pb.${entityName}CreateResponse, error) {
\tm := &${modelsPkg}.${entityName}Model{
${createModelFields}
\t}

\tcreated, err := s.repo.${entityName}.Create(ctx, m)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}CreateFailed)
\t}

\tinfo, err := created.ToPB()
\tif err != nil {
\t\treturn nil, err
\t}
\treturn &pb.${entityName}CreateResponse{${pbType}: info}, nil
}

func (s *ServiceImpl) ${entityName}Get(ctx context.Context, req *pb.${entityName}GetRequest) (*pb.${entityName}GetResponse, error) {
\tm, err := s.repo.${entityName}.GetByID(ctx, req.${primaryKeyField})
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}NotFound)
\t}

\tinfo, err := m.ToPB()
\tif err != nil {
\t\treturn nil, err
\t}
\treturn &pb.${entityName}GetResponse{${pbType}: info}, nil
}

func (s *ServiceImpl) ${entityName}List(ctx context.Context, req *pb.${entityName}ListRequest) (*pb.${entityName}ListResponse, error) {
\tquery := s.build${entityName}Query(req)
\tpagination := &repository.Pagination32{
\t\tPage:     req.PageRequest.GetPage(),
\t\tPageSize: req.PageRequest.GetSize(),
\t}

\tlist, result, err := s.repo.${entityName}.ListWithPagination(ctx, query, pagination)
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCodeDatabaseOperationFailed)
\t}

\titems := make([]*${pbPackageAlias}.${pbType}, 0, len(list))
\tfor _, m := range list {
\t\tinfo, convErr := m.ToPB()
\t\tif convErr != nil {
\t\t\treturn nil, convErr
\t\t}
\t\titems = append(items, info)
\t}

\treturn &pb.${entityName}ListResponse{
\t\tPageResponse: ${modelsPkg}.BuildPagination(req.PageRequest.GetPage(), req.PageRequest.GetSize(), int32(result.Total)),
\t\t${pbType}s:   items,
\t}, nil
}

func (s *ServiceImpl) ${entityName}Update(ctx context.Context, req *pb.${entityName}UpdateRequest) (*pb.${entityName}UpdateResponse, error) {
\tupdates := pbmo.NewUpdates().
${updateBuilder}

\tif !updates.IsEmpty() {
\t\tif err := s.repo.${entityName}.UpdateBy${primaryKeyField}(ctx, req.${primaryKeyField}, updates.Build()); err != nil {
\t\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}UpdateFailed)
\t\t}
\t}

\tm, err := s.repo.${entityName}.GetByID(ctx, req.${primaryKeyField})
\tif err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}NotFound)
\t}
\tinfo, err := m.ToPB()
\tif err != nil {
\t\treturn nil, err
\t}
\treturn &pb.${entityName}UpdateResponse{${pbType}: info}, nil
}

func (s *ServiceImpl) ${entityName}Delete(ctx context.Context, req *pb.${entityName}DeleteRequest) (*pb.${entityName}DeleteResponse, error) {
\tif err := s.repo.${entityName}.DeleteByID(ctx, req.${primaryKeyField}); err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}DeleteFailed)
\t}
\treturn &pb.${entityName}DeleteResponse{}, nil
}
${hasStatus ? `
func (s *ServiceImpl) ${entityName}UpdateStatus(ctx context.Context, req *pb.${entityName}UpdateStatusRequest) (*pb.${entityName}UpdateStatusResponse, error) {
\tif err := s.repo.${entityName}.UpdateStatus(ctx, req.${primaryKeyField}, int32(req.Status)); err != nil {
\t\treturn nil, errors.ToGRPCError(ctx, errors.BizErrCode${entityName}UpdateStatusFailed)
\t}
\treturn &pb.${entityName}UpdateStatusResponse{}, nil
}` : ''}

func (s *ServiceImpl) build${entityName}Query(req *pb.${entityName}ListRequest) *repository.Query {
\tq := repository.NewQuery().
\t\tWithPaging(int(req.PageRequest.GetPage()), int(req.PageRequest.GetSize())).
\t\tAddSafeOrder("", "", "created_at", "DESC")${listQueryFilters}
\treturn q
}`;
}

function protoNameFromGoName(goName: string): string {
  return goName.charAt(0).toUpperCase() + goName.slice(1);
}

function buildListQueryFilters(config: GeneratorConfig): string {
  const filterFields = config.fields.filter(f =>
    !f.isPrimaryKey && !f.isCreatedAt && !f.isUpdatedAt &&
    (f.goType === 'string' || f.isEnum)
  );

  if (filterFields.length === 0) {
    return '';
  }

  const lines = filterFields.map(f => {
    const snakeName = camelToSnake(f.goName);
    const reqField = protoNameFromGoName(f.goName);
    if (f.isEnum) {
      return `\n\t\tAddFilterIfNotEmpty("${snakeName}", req.${reqField})`;
    }
    if (f.goName === 'Name' || f.goName.includes('Name')) {
      return `\n\t\tAddLikeFilterIfNotEmpty("${snakeName}", req.${reqField}.GetValue())`;
    }
    return `\n\t\tAddFilterIfNotEmpty("${snakeName}", req.${reqField}.GetValue())`;
  });

  return lines.join('');
}
