export interface ProtoField {
  name: string;
  type: string;
  number: number;
  repeated: boolean;
  optional: boolean;
  comment: string;
}

export interface ProtoMessage {
  name: string;
  fields: ProtoField[];
  comment: string;
}

export interface ProtoService {
  name: string;
  methods: ProtoMethod[];
}

export interface ProtoMethod {
  name: string;
  inputType: string;
  outputType: string;
}

export interface ParsedProto {
  package: string;
  goPackage: string;
  messages: ProtoMessage[];
  services: ProtoService[];
}

export interface GoModelField {
  name: string;
  goType: string;
  gormTag: string;
  jsonTag: string;
  comment: string;
  isPrimaryKey: boolean;
}

export interface GoModelStruct {
  name: string;
  tableName: string;
  tableComment: string;
  fields: GoModelField[];
  primaryKeyField: string;
  primaryKeyType: string;
  pbType: string;
  pbPackage: string;
  enumPackage: string;
}

export interface GeneratedCode {
  label: string;
  language: string;
  code: string;
  fileName: string;
}

export interface GeneratorConfig {
  serviceName: string;
  moduleName: string;
  pbPackage: string;
  pbPackageAlias: string;
  enumPackageAlias: string;
  entityName: string;
  entityNameLower: string;
  pbType: string;
  primaryKeyField: string;
  primaryKeyType: string;
  tableName: string;
  tableComment: string;
  serviceStyle: 'single' | 'separate';
  fields: FieldConfig[];
  description?: string;
  enableFileHeader?: boolean;
  author?: string;
  publisher?: string;
  filePath?: string;
  modelsPackageName?: string;
  repositoryPackageName?: string;
  servicePackageName?: string;
  errorsPackageName?: string;
  modelsImportPath?: string;
  repositoryImportPath?: string;
  serviceImportPath?: string;
  errorsImportPath?: string;
  outputDir?: string;
}

export interface FieldConfig {
  protoName: string;
  goName: string;
  goType: string;
  gormTag: string;
  jsonTag: string;
  comment: string;
  isPrimaryKey: boolean;
  isEnum: boolean;
  enumType: string;
  protoType: string;
  isUpdatedAt: boolean;
  isCreatedAt: boolean;
  isI18N: boolean;
  isOptionalWrapper: boolean;
}
