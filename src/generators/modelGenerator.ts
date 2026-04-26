import { GeneratorConfig, FieldConfig } from '../types';
import { camelToSnake, isTimestampField, generateFileHeader } from './index';

export function generateModel(config: GeneratorConfig): string {
  const { entityName, tableName, tableComment, pbPackageAlias, pbType, fields } = config;
  const pkgName = config.modelsPackageName || 'models';

  const structFields = fields.map(f => modelFieldLine(f, pbPackageAlias)).join('\n');
  const hasTime = fields.some(f => isTimestampField(f));
  const hasEnum = fields.some(f => f.isEnum);
  const hasI18N = fields.some(f => f.isI18N);

  const imports = buildModelImports(config, hasTime, hasEnum, hasI18N);
  const header = generateFileHeader(config, '模型定义');

  return `${header}
package ${pkgName}

${imports}

// ${entityName}Model ${tableComment || entityName}模型 - 字段名称与proto保持一致以支持PBMO自动转换
type ${entityName}Model struct {
${structFields}
}

// TableName 指定表名
func (${entityName}Model) TableName() string {
\treturn "${tableName}"
}

// TableComment 指定表注释
func (${entityName}Model) TableComment() string {
\treturn "${tableComment}"
}

// NewConverter 注册PBMO转换器
func (${entityName}Model) NewConverter() *pbmo.BidiConverter {
\treturn pbmo.Register[${entityName}Model, ${pbPackageAlias}.${pbType}]()
}

// ToPB 将模型转换为PB消息
func (m *${entityName}Model) ToPB() (*${pbPackageAlias}.${pbType}, error) {
\treturn pbmo.ToPB[${entityName}Model, ${pbPackageAlias}.${pbType}](m)
}`;
}

function modelFieldLine(field: FieldConfig, pbPackageAlias: string): string {
  const snakeName = camelToSnake(field.goName);
  let gormTag = field.gormTag;
  if (!gormTag) {
    gormTag = inferGormTag(field);
  }
  const jsonTag = field.jsonTag || snakeName;
  const comment = field.comment ? `\t// ${field.goName} ${field.comment}` : '';

  return `${comment}\n\t${field.goName}\t${field.goType}\t\`gorm:"${gormTag}" json:"${jsonTag}"\``;
}

function inferGormTag(field: FieldConfig): string {
  const snakeName = camelToSnake(field.goName);
  const parts: string[] = [`column:${snakeName}`];

  if (field.isPrimaryKey) {
    if (field.goType === 'string') {
      parts.push('type:varchar(36)', 'primaryKey');
    } else {
      parts.push('primaryKey', 'autoIncrement');
    }
  } else if (field.goType === 'string') {
    if (snakeName.includes('description') || snakeName.includes('config')) {
      parts.push('type:text');
    } else if (snakeName.includes('email') || snakeName.includes('account')) {
      parts.push('type:varchar(128)');
    } else if (snakeName.length > 32) {
      parts.push('type:varchar(256)');
    } else {
      parts.push('type:varchar(64)');
    }
    parts.push("default:''");
  } else if (field.goType === 'int32' || field.goType === 'int64') {
    parts.push('type:int', 'default:0');
  } else if (field.goType === 'bool') {
    parts.push('type:tinyint', 'default:0');
  } else if (isTimestampField(field)) {
    if (field.goName === 'CreatedAt') {
      parts.push('autoCreateTime');
    } else if (field.goName === 'UpdatedAt') {
      parts.push('autoUpdateTime');
    }
  }

  if (snakeName.includes('tenant_id') || (snakeName.endsWith('_id') && !field.isPrimaryKey)) {
    parts.push('index');
  }

  parts.push(`comment:${field.comment || field.goName}`);

  return parts.join(';');
}

function buildModelImports(config: GeneratorConfig, hasTime: boolean, hasEnum: boolean, hasI18N: boolean): string {
  const imports: string[] = [];

  imports.push(`\tpb "${config.pbPackage}"`);
  imports.push('\t"github.com/kamalyes/go-pbmo"');

  if (hasEnum) {
    imports.push(`\t${config.enumPackageAlias}`);
  }

  if (hasTime) {
    imports.push('\t"time"');
  }

  if (imports.length > 0) {
    return `import (\n${imports.join('\n')}\n)`;
  }

  return '';
}
