import { DatabaseConfig, ColumnInfo, TableInfo } from './types';
import * as vscode from 'vscode';
import * as mysql from 'mysql2/promise';
import * as pg from 'pg';

const OUTPUT_CHANNEL_NAME = 'Kronos CodeGen';
let _outputChannel: vscode.OutputChannel | undefined;

function getOutputChannel(): vscode.OutputChannel {
  if (!_outputChannel) {
    _outputChannel = vscode.window.createOutputChannel(OUTPUT_CHANNEL_NAME);
  }
  return _outputChannel;
}

export function log(msg: string) {
  getOutputChannel().appendLine(`[${new Date().toLocaleTimeString()}] ${msg}`);
}

export function logError(msg: string, err?: any) {
  const detail = err ? (err.message || String(err)) : '';
  getOutputChannel().appendLine(`[${new Date().toLocaleTimeString()}] ❌ ${msg} ${detail}`);
  if (err && err.code) {
    getOutputChannel().appendLine(`  code: ${err.code}`);
  }
  if (err && err.errno) {
    getOutputChannel().appendLine(`  errno: ${err.errno}`);
  }
}

export function showOutput() {
  getOutputChannel().show(true);
}

export async function fetchTableList(config: DatabaseConfig): Promise<string[]> {
  if (config.type === 'mysql') {
    return fetchMySQLTableList(config);
  } else {
    return fetchPostgreSQLTableList(config);
  }
}

export async function fetchTableStructure(config: DatabaseConfig, tableName: string): Promise<TableInfo> {
  if (config.type === 'mysql') {
    return fetchMySQLTableStructure(config, tableName);
  } else {
    return fetchPostgreSQLTableStructure(config, tableName);
  }
}

export async function fetchMultipleTableStructures(config: DatabaseConfig, tableNames: string[]): Promise<TableInfo[]> {
  const results: TableInfo[] = [];
  for (const tableName of tableNames) {
    const tableInfo = await fetchTableStructure(config, tableName);
    results.push(tableInfo);
  }
  return results;
}

async function fetchMySQLTableList(config: DatabaseConfig): Promise<string[]> {
  const sql = `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME`;
  log(`MySQL 查询表列表: ${sql.replace('?', config.database)}`);

  let conn: mysql.Connection | undefined;
  try {
    conn = await mysql.createConnection({
      host: config.host,
      port: config.port,
      user: config.username,
      password: config.password,
      database: config.database,
      connectTimeout: 15000,
    });

    const [rows] = await conn.execute(sql, [config.database]) as [any[], any];
    const tableNames = rows.map((r: any) => r.TABLE_NAME);
    log(`MySQL 返回 ${tableNames.length} 张表`);
    return tableNames;
  } catch (err) {
    logError('MySQL 获取表列表失败', err);
    throw err;
  } finally {
    if (conn) { await conn.end().catch(() => {}); }
  }
}

async function fetchPostgreSQLTableList(config: DatabaseConfig): Promise<string[]> {
  const sql = `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename`;
  log(`PostgreSQL 查询表列表: ${sql}`);

  const client = new pg.Client({
    host: config.host,
    port: config.port,
    user: config.username,
    password: config.password,
    database: config.database,
    connectionTimeoutMillis: 15000,
  });

  try {
    await client.connect();
    const result = await client.query(sql);
    const tableNames = result.rows.map((r: any) => r.tablename);
    log(`PostgreSQL 返回 ${tableNames.length} 张表`);
    return tableNames;
  } catch (err) {
    logError('PostgreSQL 获取表列表失败', err);
    throw err;
  } finally {
    await client.end().catch(() => {});
  }
}

async function fetchMySQLTableStructure(config: DatabaseConfig, tableName: string): Promise<TableInfo> {
  let conn: mysql.Connection | undefined;
  try {
    conn = await mysql.createConnection({
      host: config.host,
      port: config.port,
      user: config.username,
      password: config.password,
      database: config.database,
      connectTimeout: 15000,
    });

    const [tableRows] = await conn.execute(
      `SELECT TABLE_COMMENT, ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
      [config.database, tableName]
    ) as [any[], any];

    const tableInfo = (tableRows as any[])[0] || {};
    const tableComment = tableInfo.TABLE_COMMENT || '';
    const engine = tableInfo.ENGINE || '';

    const [columnRows] = await conn.execute(
      `SELECT COLUMN_NAME, COLUMN_TYPE, DATA_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, EXTRA, COLUMN_COMMENT, CHARACTER_MAXIMUM_LENGTH, ORDINAL_POSITION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`,
      [config.database, tableName]
    ) as [any[], any];

    const columns = (columnRows as any[]).map(row => ({
      columnName: row.COLUMN_NAME,
      columnType: row.COLUMN_TYPE,
      dataType: row.DATA_TYPE,
      isNullable: row.IS_NULLABLE === 'YES',
      columnKey: row.COLUMN_KEY,
      columnDefault: row.COLUMN_DEFAULT,
      extra: row.EXTRA || '',
      columnComment: row.COLUMN_COMMENT || '',
      maxLength: row.CHARACTER_MAXIMUM_LENGTH,
      ordinalPosition: row.ORDINAL_POSITION,
      isPrimaryKey: row.COLUMN_KEY === 'PRI',
      isAutoIncrement: (row.EXTRA || '').includes('auto_increment'),
    }));

    return { tableName, tableComment, columns, engine };
  } finally {
    if (conn) { await conn.end().catch(() => {}); }
  }
}

async function fetchPostgreSQLTableStructure(config: DatabaseConfig, tableName: string): Promise<TableInfo> {
  const client = new pg.Client({
    host: config.host,
    port: config.port,
    user: config.username,
    password: config.password,
    database: config.database,
    connectionTimeoutMillis: 15000,
  });

  try {
    await client.connect();

    const commentResult = await client.query(
      `SELECT obj_description($1::regclass, 'pg_class') as comment`,
      [tableName]
    );
    const tableComment = (commentResult.rows[0]?.comment || '') as string;

    const columnResult = await client.query(
      `SELECT c.column_name, c.udt_name, c.data_type, c.is_nullable,
        CASE WHEN pk.column_name IS NOT NULL THEN 'PRI' ELSE '' END as column_key,
        c.column_default,
        CASE WHEN c.column_default LIKE 'nextval%' THEN 'auto_increment' ELSE '' END as extra,
        COALESCE(pgd.description, '') as column_comment,
        c.character_maximum_length,
        c.ordinal_position
      FROM information_schema.columns c
      LEFT JOIN (
        SELECT ku.column_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage ku ON tc.constraint_name = ku.constraint_name
        WHERE tc.table_name = $1 AND tc.constraint_type = 'PRIMARY KEY'
      ) pk ON c.column_name = pk.column_name
      LEFT JOIN pg_catalog.pg_statio_all_tables st ON st.relname = $1
      LEFT JOIN pg_catalog.pg_description pgd ON pgd.objoid = st.relid AND pgd.objsubid = c.ordinal_position
      WHERE c.table_name = $1 AND c.table_schema = 'public'
      ORDER BY c.ordinal_position`,
      [tableName]
    );

    const columns = columnResult.rows.map(row => {
      const udtName = row.udt_name as string;
      const maxLength = row.character_maximum_length as number | null;
      return {
        columnName: row.column_name as string,
        columnType: mapPostgreSQLType(udtName, maxLength),
        dataType: (row.data_type as string) || udtName,
        isNullable: row.is_nullable === 'YES',
        columnKey: row.column_key as string,
        columnDefault: row.column_default as string | null,
        extra: row.extra as string,
        columnComment: row.column_comment as string,
        maxLength,
        ordinalPosition: row.ordinal_position as number,
        isPrimaryKey: row.column_key === 'PRI',
        isAutoIncrement: (row.extra as string).includes('auto_increment'),
      };
    });

    return { tableName, tableComment, columns, engine: 'postgresql' };
  } finally {
    await client.end().catch(() => {});
  }
}

function mapPostgreSQLType(udtName: string, maxLength: number | null): string {
  const typeMap: Record<string, string> = {
    'int4': 'int',
    'int8': 'bigint',
    'int2': 'smallint',
    'varchar': maxLength ? `varchar(${maxLength})` : 'varchar',
    'bpchar': 'char',
    'text': 'text',
    'bool': 'tinyint(1)',
    'float4': 'float',
    'float8': 'double',
    'numeric': 'decimal',
    'timestamp': 'timestamp',
    'timestamptz': 'timestamp',
    'date': 'date',
    'time': 'time',
    'json': 'json',
    'jsonb': 'json',
    'uuid': 'varchar(36)',
    'bytea': 'blob',
  };
  return typeMap[udtName] || udtName;
}

export async function testConnection(config: DatabaseConfig): Promise<boolean> {
  if (config.type === 'mysql') {
    log(`测试 MySQL 连接: ${config.host}:${config.port}/${config.database}`);
    let conn: mysql.Connection | undefined;
    try {
      conn = await mysql.createConnection({
        host: config.host,
        port: config.port,
        user: config.username,
        password: config.password,
        database: config.database,
        connectTimeout: 10000,
      });
      await conn.execute('SELECT 1');
      log('MySQL 连接测试成功');
      return true;
    } catch (err) {
      logError('MySQL 连接测试失败', err);
      return false;
    } finally {
      if (conn) { await conn.end().catch(() => {}); }
    }
  } else {
    log(`测试 PostgreSQL 连接: ${config.host}:${config.port}/${config.database}`);
    const client = new pg.Client({
      host: config.host,
      port: config.port,
      user: config.username,
      password: config.password,
      database: config.database,
      connectionTimeoutMillis: 10000,
    });
    try {
      await client.connect();
      await client.query('SELECT 1');
      log('PostgreSQL 连接测试成功');
      return true;
    } catch (err) {
      logError('PostgreSQL 连接测试失败', err);
      return false;
    } finally {
      await client.end().catch(() => {});
    }
  }
}
