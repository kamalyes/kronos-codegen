import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { DatabaseConfig } from './types';

const CONFIG_KEY = 'kronos-codegen.databaseConnections';

function stripJsonComments(jsonStr: string): string {
  return jsonStr.replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');
}

function getWorkspaceRoot(): string | undefined {
  const workspaceFolders = vscode.workspace.workspaceFolders;
  if (!workspaceFolders || workspaceFolders.length === 0) {
    return undefined;
  }
  return workspaceFolders[0].uri.fsPath;
}

function readSettingsFile(settingsPath: string): { [key: string]: any } {
  try {
    if (fs.existsSync(settingsPath)) {
      const content = fs.readFileSync(settingsPath, 'utf-8');
      return JSON.parse(stripJsonComments(content));
    }
  } catch {
    // ignore parse errors
  }
  return {};
}

function writeSettingsFile(settingsPath: string, json: { [key: string]: any }): void {
  const dir = path.dirname(settingsPath);
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
  fs.writeFileSync(settingsPath, JSON.stringify(json, null, 2) + '\n', 'utf-8');
}

function findSettingsFilePath(): string | undefined {
  const root = getWorkspaceRoot();
  if (!root) { return undefined; }

  const traePath = path.join(root, '.trae', 'settings.json');
  if (fs.existsSync(traePath)) { return traePath; }

  const vscodePath = path.join(root, '.vscode', 'settings.json');
  if (fs.existsSync(vscodePath)) { return vscodePath; }

  return path.join(root, '.vscode', 'settings.json');
}

export function getSavedConnections(): DatabaseConfig[] {
  const config = vscode.workspace.getConfiguration('kronos-codegen');
  const apiConnections = config.get<DatabaseConfig[]>(CONFIG_KEY, []);
  if (apiConnections.length > 0) {
    return apiConnections;
  }

  const root = getWorkspaceRoot();
  if (!root) { return []; }

  for (const settingsDir of ['.trae', '.vscode']) {
    const settingsPath = path.join(root, settingsDir, 'settings.json');
    const json = readSettingsFile(settingsPath);
    if (json[CONFIG_KEY] && Array.isArray(json[CONFIG_KEY])) {
      return json[CONFIG_KEY] as DatabaseConfig[];
    }
  }

  return [];
}

export async function saveConnection(dbConfig: DatabaseConfig): Promise<void> {
  const connections = getSavedConnections();
  const existingIdx = connections.findIndex(c => c.name === dbConfig.name);
  if (existingIdx >= 0) {
    connections[existingIdx] = dbConfig;
  } else {
    connections.push(dbConfig);
  }

  const settingsPath = findSettingsFilePath();
  if (!settingsPath) {
    vscode.window.showWarningMessage('未找到工作区，无法保存连接配置');
    return;
  }

  const json = readSettingsFile(settingsPath);
  json[CONFIG_KEY] = connections;
  writeSettingsFile(settingsPath, json);
}

export async function deleteConnection(name: string): Promise<void> {
  const connections = getSavedConnections().filter(c => c.name !== name);

  const settingsPath = findSettingsFilePath();
  if (!settingsPath) { return; }

  const json = readSettingsFile(settingsPath);
  json[CONFIG_KEY] = connections;
  writeSettingsFile(settingsPath, json);
}

export async function promptForDatabaseConfig(): Promise<DatabaseConfig | undefined> {
  const savedConnections = getSavedConnections();

  if (savedConnections.length > 0) {
    const choices = [
      ...savedConnections.map(c => ({
        label: `📡 ${c.name}`,
        description: `${c.type}://${c.host}:${c.port}/${c.database}`,
        connection: c,
      })),
      { label: '➕ 新建连接', description: '配置新的数据库连接', connection: null },
    ];

    const selected = await vscode.window.showQuickPick(choices, {
      placeHolder: '选择数据库连接或新建',
    });

    if (!selected) { return undefined; }

    if (selected.connection) {
      return selected.connection;
    }
  }

  const dbType = await vscode.window.showQuickPick(
    [
      { label: 'MySQL', value: 'mysql' as const },
      { label: 'PostgreSQL', value: 'postgresql' as const },
    ],
    { placeHolder: '选择数据库类型' }
  );
  if (!dbType) { return undefined; }

  const host = await vscode.window.showInputBox({
    prompt: '数据库主机地址',
    value: '127.0.0.1',
  });
  if (!host) { return undefined; }

  const portStr = await vscode.window.showInputBox({
    prompt: '数据库端口',
    value: dbType.value === 'mysql' ? '3306' : '5432',
  });
  if (!portStr) { return undefined; }

  const username = await vscode.window.showInputBox({
    prompt: '数据库用户名',
    value: 'root',
  });
  if (!username) { return undefined; }

  const password = await vscode.window.showInputBox({
    prompt: '数据库密码',
    password: true,
  });
  if (password === undefined) { return undefined; }

  const database = await vscode.window.showInputBox({
    prompt: '数据库名称',
  });
  if (!database) { return undefined; }

  const name = await vscode.window.showInputBox({
    prompt: '连接名称（用于保存）',
    value: `${dbType.label}-${database}`,
  });
  if (!name) { return undefined; }

  const config: DatabaseConfig = {
    type: dbType.value,
    host,
    port: parseInt(portStr, 10),
    username,
    password,
    database,
    name,
  };

  const shouldSave = await vscode.window.showQuickPick(
    ['是，保存连接', '否，仅本次使用'],
    { placeHolder: '是否保存此连接配置？' }
  );

  if (shouldSave && shouldSave.startsWith('是')) {
    await saveConnection(config);
    vscode.window.showInformationMessage(`数据库连接 "${name}" 已保存`);
  }

  return config;
}
