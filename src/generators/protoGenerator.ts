import { TableInfo, ColumnInfo } from '../database/types';
import { GeneratedCode } from '../types';

export interface ProtoGeneratorConfig {
  packageName: string;
  goPackage: string;
  javaPackage: string;
  tables: TableInfo[];
  serviceName: string;
  includeCRUD: boolean;
}

export function generateProtoFromTables(config: ProtoGeneratorConfig): GeneratedCode {
  const parts: string[] = [];

  parts.push(generateProtoHeader(config));
  parts.push('');

  for (const table of config.tables) {
    parts.push(generateProtoMessageFromTable(table));
    parts.push('');
  }

  if (config.includeCRUD) {
    for (const table of config.tables) {
      parts.push(generateCRUDMessages(table));
      parts.push('');
    }

    parts.push(generateProtoService(config));
  }

  return {
    label: 'Proto 定义',
    language: 'protobuf',
    code: parts.join('\n'),
    fileName: `${config.packageName}.proto`,
  };
}

function generateProtoHeader(config: ProtoGeneratorConfig): string {
  return `syntax = "proto3";

package ${config.packageName};

option go_package = "${config.goPackage}";
option java_package = "${config.javaPackage}";

import "google/protobuf/timestamp.proto";
import "google/protobuf/wrappers.proto";
import "enums/enums.proto";`;
}

function generateProtoMessageFromTable(table: TableInfo): string {
  const entityName = tableNameToEntityName(table.tableName);
  const comment = table.tableComment ? ` // ${table.tableComment}` : '';

  const fields = table.columns.map((col, idx) => {
    const protoType = dbTypeToProtoType(col);
    const fieldName = columnNameToProtoName(col.columnName);
    const fieldComment = col.columnComment ? ` // ${col.columnComment}` : '';
    return `  ${protoType.prefix}${protoType.type} ${fieldName} = ${idx + 1};${fieldComment}`;
  });

  return `message ${entityName}Info {${comment}
${fields.join('\n')}
}`;
}

function generateCRUDMessages(table: TableInfo): string {
  const entityName = tableNameToEntityName(table.tableName);
  const pkField = findPrimaryKey(table.columns);
  const pkProtoName = pkField ? columnNameToProtoName(pkField.columnName) : 'id';
  const pkProtoType = pkField ? dbTypeToProtoType(pkField).type : 'string';

  const createFields = table.columns
    .filter(c => !c.isAutoIncrement && !c.isPrimaryKey)
    .map((col, idx) => {
      const protoType = dbTypeToProtoType(col);
      const fieldName = columnNameToProtoName(col.columnName);
      return `  ${protoType.prefix}${protoType.type} ${fieldName} = ${idx + 1};`;
    });

  const updateFields = table.columns
    .filter(c => !c.isAutoIncrement && !c.isPrimaryKey && !isTimestampColumn(c))
    .map((col, idx) => {
      const protoType = dbTypeToProtoType(col, true);
      const fieldName = columnNameToProtoName(col.columnName);
      return `  ${protoType.prefix}${protoType.type} ${fieldName} = ${idx + 1};`;
    });

  const filterFields = table.columns
    .filter(c => !c.isAutoIncrement && !isTimestampColumn(c) && isFilterableColumn(c))
    .map((col, idx) => {
      const protoType = dbTypeToProtoType(col, true);
      const fieldName = columnNameToProtoName(col.columnName);
      return `  ${protoType.prefix}${protoType.type} ${fieldName} = ${idx + 1};`;
    });

  return `message ${entityName}CreateRequest {
${createFields.join('\n')}
}

message ${entityName}CreateResponse {
  ${entityName}Info ${entityName.charAt(0).toLowerCase() + entityName.slice(1)}_info = 1;
}

message ${entityName}GetRequest {
  ${pkProtoType} ${pkProtoName} = 1;
}

message ${entityName}GetResponse {
  ${entityName}Info ${entityName.charAt(0).toLowerCase() + entityName.slice(1)}_info = 1;
}

message ${entityName}UpdateRequest {
  ${pkProtoType} ${pkProtoName} = 1;
${updateFields.map((f, i) => `  ${f.trim()}`).join('\n')}
}

message ${entityName}UpdateResponse {
  ${entityName}Info ${entityName.charAt(0).toLowerCase() + entityName.slice(1)}_info = 1;
}

message ${entityName}DeleteRequest {
  ${pkProtoType} ${pkProtoName} = 1;
}

message ${entityName}DeleteResponse {
}

message ${entityName}ListRequest {
${filterFields.join('\n')}
  int32 page = 50;
  int32 page_size = 51;
}

message ${entityName}ListResponse {
  repeated ${entityName}Info ${entityName.charAt(0).toLowerCase() + entityName.slice(1)}_infos = 1;
  int32 total = 2;
}`;
}

function generateProtoService(config: ProtoGeneratorConfig): string {
  const methods: string[] = [];

  for (const table of config.tables) {
    const entityName = tableNameToEntityName(table.tableName);
    const lowerName = entityName.charAt(0).toLowerCase() + entityName.slice(1);

    methods.push(`  rpc ${entityName}Create(${entityName}CreateRequest) returns (${entityName}CreateResponse);`);
    methods.push(`  rpc ${entityName}Get(${entityName}GetRequest) returns (${entityName}GetResponse);`);
    methods.push(`  rpc ${entityName}List(${entityName}ListRequest) returns (${entityName}ListResponse);`);
    methods.push(`  rpc ${entityName}Update(${entityName}UpdateRequest) returns (${entityName}UpdateResponse);`);
    methods.push(`  rpc ${entityName}Delete(${entityName}DeleteRequest) returns (${entityName}DeleteResponse);`);
  }

  return `service ${config.serviceName} {
${methods.join('\n')}
}`;
}

interface ProtoTypeInfo {
  type: string;
  prefix: string;
}

function dbTypeToProtoType(col: ColumnInfo, useWrapper: boolean = false): ProtoTypeInfo {
  const dt = col.dataType.toLowerCase();
  const isNullable = col.isNullable;

  const typeMap: Record<string, string> = {
    'varchar': 'string',
    'char': 'string',
    'text': 'string',
    'mediumtext': 'string',
    'longtext': 'string',
    'tinytext': 'string',
    'int': 'int32',
    'integer': 'int32',
    'tinyint': 'int32',
    'smallint': 'int32',
    'mediumint': 'int32',
    'bigint': 'int64',
    'float': 'float',
    'double': 'double',
    'decimal': 'string',
    'numeric': 'string',
    'bool': 'bool',
    'boolean': 'bool',
    'date': 'string',
    'datetime': 'google.protobuf.Timestamp',
    'timestamp': 'google.protobuf.Timestamp',
    'time': 'string',
    'blob': 'bytes',
    'tinyblob': 'bytes',
    'mediumblob': 'bytes',
    'longblob': 'bytes',
    'binary': 'bytes',
    'json': 'string',
    'jsonb': 'string',
    'uuid': 'string',
    'enum': 'string',
    'set': 'string',
    'bit': 'int32',
    'point': 'string',
    'geometry': 'string',
  };

  let protoType = typeMap[dt] || 'string';

  if (useWrapper && isNullable) {
    if (protoType === 'string') {
      protoType = 'google.protobuf.StringValue';
    } else if (protoType === 'int32') {
      protoType = 'google.protobuf.Int32Value';
    } else if (protoType === 'int64') {
      protoType = 'google.protobuf.Int64Value';
    } else if (protoType === 'bool') {
      protoType = 'google.protobuf.BoolValue';
    }
  }

  if (col.columnName.endsWith('_id') || col.columnName === 'id') {
    protoType = 'string';
  }

  if (col.columnName.endsWith('_status') || col.columnName === 'status') {
    protoType = 'enums.CommonStatus';
  }

  if (col.columnName.endsWith('_type') && col.columnName !== 'column_type') {
    protoType = 'int32';
  }

  return {
    type: protoType,
    prefix: '',
  };
}

function tableNameToEntityName(tableName: string): string {
  let name = tableName.replace(/^(t_|tb_|tbl_)/, '');
  return name
    .split('_')
    .filter(part => part.length > 0)
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join('')
    .replace(/s$/, '');
}

function columnNameToProtoName(columnName: string): string {
  return columnName.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

function findPrimaryKey(columns: ColumnInfo[]): ColumnInfo | undefined {
  return columns.find(c => c.isPrimaryKey) || columns[0];
}

function isTimestampColumn(col: ColumnInfo): boolean {
  const dt = col.dataType.toLowerCase();
  return dt === 'datetime' || dt === 'timestamp' ||
         col.columnName === 'created_at' || col.columnName === 'updated_at';
}

function isFilterableColumn(col: ColumnInfo): boolean {
  const dt = col.dataType.toLowerCase();
  return ['varchar', 'int', 'tinyint', 'smallint', 'bigint', 'enum', 'bool', 'boolean'].includes(dt) ||
         col.columnName.endsWith('_id') ||
         col.columnName.endsWith('_status') ||
         col.columnName.endsWith('_type');
}

export function tableInfoToFieldConfigs(table: TableInfo): import('../types').FieldConfig[] {
  return table.columns.map(col => {
    const goName = columnNameToProtoName(col.columnName).charAt(0).toUpperCase() +
                   columnNameToProtoName(col.columnName).slice(1);
    const goType = dbTypeToGoType(col);
    const isEnum = col.columnName.endsWith('_status') || col.columnName === 'status';

    return {
      protoName: columnNameToProtoName(col.columnName),
      goName,
      goType: isEnum ? `enumspb.${goName}` : goType,
      gormTag: buildGormTagFromColumn(col),
      jsonTag: columnNameToProtoName(col.columnName),
      comment: col.columnComment || col.columnName,
      isPrimaryKey: col.isPrimaryKey,
      isEnum,
      enumType: isEnum ? 'CommonStatus' : '',
      protoType: dbTypeToProtoType(col).type,
      isUpdatedAt: col.columnName === 'updated_at',
      isCreatedAt: col.columnName === 'created_at',
      isI18N: false,
      isOptionalWrapper: col.isNullable,
    };
  });
}

function dbTypeToGoType(col: ColumnInfo): string {
  const dt = col.dataType.toLowerCase();

  if (col.columnName.endsWith('_id') || col.columnName === 'id') {
    return 'string';
  }

  const typeMap: Record<string, string> = {
    'varchar': 'string',
    'char': 'string',
    'text': 'string',
    'mediumtext': 'string',
    'longtext': 'string',
    'tinytext': 'string',
    'int': 'int32',
    'integer': 'int32',
    'tinyint': 'int32',
    'smallint': 'int32',
    'mediumint': 'int32',
    'bigint': 'int64',
    'float': 'float32',
    'double': 'float64',
    'decimal': 'string',
    'numeric': 'string',
    'bool': 'bool',
    'boolean': 'bool',
    'datetime': 'time.Time',
    'timestamp': 'time.Time',
    'date': 'time.Time',
    'time': 'string',
    'blob': '[]byte',
    'json': 'string',
    'jsonb': 'string',
    'uuid': 'string',
    'enum': 'string',
  };

  return typeMap[dt] || 'string';
}

function buildGormTagFromColumn(col: ColumnInfo): string {
  const parts: string[] = [`column:${col.columnName}`];

  if (col.isPrimaryKey) {
    parts.push('primaryKey');
    if (col.isAutoIncrement) {
      parts.push('autoIncrement');
    } else {
      parts.push('type:varchar(36)');
    }
  } else {
    parts.push(`type:${col.columnType}`);
    if (col.columnDefault !== null && col.columnDefault !== undefined) {
      if (col.columnDefault === '') {
        parts.push("default:''");
      } else {
        parts.push(`default:${col.columnDefault}`);
      }
    }
  }

  if (col.columnName.endsWith('_id') && !col.isPrimaryKey) {
    parts.push('index');
  }

  if (col.columnComment) {
    parts.push(`comment:${col.columnComment}`);
  }

  return parts.join(';');
}
