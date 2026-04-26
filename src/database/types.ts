export interface DatabaseConfig {
  type: 'mysql' | 'postgresql';
  host: string;
  port: number;
  username: string;
  password: string;
  database: string;
  name: string;
}

export interface ColumnInfo {
  columnName: string;
  columnType: string;
  dataType: string;
  isNullable: boolean;
  columnKey: string;
  columnDefault: string | null;
  extra: string;
  columnComment: string;
  maxLength: number | null;
  ordinalPosition: number;
  isPrimaryKey: boolean;
  isAutoIncrement: boolean;
}

export interface TableInfo {
  tableName: string;
  tableComment: string;
  columns: ColumnInfo[];
  engine: string;
}

export interface ForeignKeyInfo {
  columnName: string;
  referencedTable: string;
  referencedColumnName: string;
}
