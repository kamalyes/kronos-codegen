import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { parseProtoContent, findCrudMessages, protoTypeToGoType, inferGormTag } from './parsers/protoParser';
import { parseGoModelContent, modelToFieldConfigs, findAllModelStructs, findStructNames } from './parsers/goModelParser';
import { generateAll } from './generators';
import { generateProtoFromTables, tableInfoToFieldConfigs, ProtoGeneratorConfig } from './generators/protoGenerator';
import { CodePreviewPanel } from './webview/codePreviewPanel';
import { GeneratorConfig, FieldConfig, ProtoMessage } from './types';
import { detectWorkspaceInfo, findGoModInWorkspace } from './workspaceDetector';

export function activate(context: vscode.ExtensionContext) {
  try {
    registerProtoCommand(context);
    registerModelCommand(context);
    registerDatabaseCommand(context);
    registerDatabaseProtoCommand(context);
  } catch (err) {
    vscode.window.showErrorMessage(`Kronos CodeGen 激活失败: ${err}`);
  }
}

export function deactivate() {}

function registerProtoCommand(context: vscode.ExtensionContext) {
  context.subscriptions.push(
    vscode.commands.registerCommand('kronos-codegen.generateFromProto', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor || editor.document.languageId !== 'proto') {
        vscode.window.showWarningMessage('请在 Proto 文件中使用此命令');
        return;
      }

      const fullText = editor.document.getText();
      const parsed = parseProtoContent(fullText);
      if (parsed.messages.length === 0) {
        vscode.window.showWarningMessage('未找到 Proto Message 定义');
        return;
      }

      const infoMessages = parsed.messages.filter(m =>
        m.name.endsWith('Info') && !m.name.includes('Request') && !m.name.includes('Response')
      );

      if (infoMessages.length === 0) {
        vscode.window.showWarningMessage('未找到 XxxInfo 类型的 Message（如 RoleInfo、TenantInfo）');
        return;
      }

      const selectedInfoName = await vscode.window.showQuickPick(
        infoMessages.map(m => m.name),
        { placeHolder: '选择要生成代码的实体' }
      );
      if (!selectedInfoName) { return; }

      const entityName = selectedInfoName.replace('Info', '');
      const crudMessages = findCrudMessages(parsed.messages, entityName);
      const infoMsg = crudMessages.info || infoMessages.find(m => m.name === selectedInfoName);
      if (!infoMsg) { return; }

      const wsInfo = detectWorkspaceInfo(editor.document.uri.fsPath);

      const serviceStyle = await vscode.window.showQuickPick(
        [
          { label: '单 Service 结构体', value: 'single' as const, description: '所有方法挂在同一个 ServiceImpl 上' },
          { label: '独立 Service 结构体', value: 'separate' as const, description: '每个领域独立 Service' },
        ],
        { placeHolder: '选择 Service 风格' }
      );
      if (!serviceStyle) { return; }

      const moduleName = await vscode.window.showInputBox({
        prompt: 'Go Module 路径（自动检测，可修改）',
        value: wsInfo?.moduleName || '',
      });
      if (!moduleName) { return; }

      const pbPackage = await vscode.window.showInputBox({
        prompt: 'PB 包导入路径（自动检测，可修改）',
        value: wsInfo?.pbPackage || '',
      });
      if (!pbPackage) { return; }

      const fields = protoMessageToFieldConfigs(infoMsg, parsed);
      const headerCfg = getFileHeaderConfig();

      const config: GeneratorConfig = {
        serviceName: inferServiceName(parsed.services, entityName),
        moduleName,
        pbPackage,
        pbPackageAlias: wsInfo?.pbPackageAlias || 'pb',
        enumPackageAlias: wsInfo?.enumPackageAlias || 'enumspb',
        entityName,
        entityNameLower: entityName.charAt(0).toLowerCase() + entityName.slice(1),
        pbType: `${entityName}Info`,
        primaryKeyField: inferPrimaryKey(infoMsg),
        primaryKeyType: 'string',
        tableName: inferTableName(entityName),
        tableComment: `${entityName}表`,
        serviceStyle: serviceStyle.value,
        fields,
        ...headerCfg,
      };

      const codes = generateAll(config);
      CodePreviewPanel.render(context.extensionUri, codes);
    })
  );
}

function registerModelCommand(context: vscode.ExtensionContext) {
  context.subscriptions.push(
    vscode.commands.registerCommand('kronos-codegen.generateFromModel', async () => {
      const editor = vscode.window.activeTextEditor;
      if (!editor || editor.document.languageId !== 'go') {
        vscode.window.showWarningMessage('请在 Go 文件中使用此命令');
        return;
      }

      const fullText = editor.document.getText();
      const selectedText = editor.document.getText(editor.selection);
      let model: import('./types').GoModelStruct | null = null;

      if (selectedText.trim()) {
        model = parseGoModelContent(fullText, selectedText);
        if (!model) {
          const structNames = findStructNames(fullText);
          const matchByName = structNames.find(n => selectedText.includes(n));
          if (matchByName) {
            const allStructs = findAllModelStructs(fullText);
            const targetStruct = allStructs.find(s => s[1] === matchByName);
            if (targetStruct) {
              model = parseGoModelContent(fullText, targetStruct[0]);
            }
          }
        }
      }

      if (!model) {
        const structNames = findStructNames(fullText);
        if (structNames.length === 0) {
          vscode.window.showWarningMessage('当前文件中未找到任何 Model 结构体定义');
          return;
        }

        const items = structNames.map((name, i) => ({
          label: name,
          description: name.replace(/Model$|Data$|Entry$|Info$|Item$|Record$/, ''),
          detail: i === 0 ? '$(check) 默认选中' : undefined,
        }));
        const picked = await showQuickPickWithDefault(items, '选择要生成代码的 Model（输入名称搜索，回车确认）');
        if (!picked) { return; }

        const allStructs = findAllModelStructs(fullText);
        const targetStruct = allStructs.find(s => s[1] === (picked as any).label);
        if (targetStruct) {
          model = parseGoModelContent(fullText, targetStruct[0]);
        }
      }

      if (!model) {
        model = parseGoModelContent(fullText);
      }

      if (!model) {
        vscode.window.showWarningMessage('未能解析 Model 结构体，请确保文件中包含 type XxxModel struct { ... } 定义');
        return;
      }

      const entityName = model.name.replace('Model', '');
      const wsInfo = detectWorkspaceInfo(editor.document.uri.fsPath);

      const serviceStyle = await vscode.window.showQuickPick(
        [
          { label: '单 Service 结构体', value: 'single' as const, description: '所有方法挂在同一个 ServiceImpl 上' },
          { label: '独立 Service 结构体', value: 'separate' as const, description: '每个领域独立 Service' },
        ],
        { placeHolder: '选择 Service 风格' }
      );
      if (!serviceStyle) { return; }

      const moduleName = await vscode.window.showInputBox({
        prompt: 'Go Module 路径（自动检测，可修改）',
        value: wsInfo?.moduleName || model.pbPackage || '',
      });
      if (!moduleName) { return; }

      const pbPackage = await vscode.window.showInputBox({
        prompt: 'PB 包导入路径（自动检测，可修改）',
        value: wsInfo?.pbPackage || model.pbPackage || '',
      });
      if (!pbPackage) { return; }

      const fields = modelToFieldConfigs(model);
      const headerCfg = getFileHeaderConfig();

      const config: GeneratorConfig = {
        serviceName: `${entityName}Service`,
        moduleName,
        pbPackage,
        pbPackageAlias: wsInfo?.pbPackageAlias || 'pb',
        enumPackageAlias: wsInfo?.enumPackageAlias || 'enumspb',
        entityName,
        entityNameLower: entityName.charAt(0).toLowerCase() + entityName.slice(1),
        pbType: model.pbType || `${entityName}Info`,
        primaryKeyField: model.primaryKeyField || inferPrimaryKeyFromFields(fields),
        primaryKeyType: model.primaryKeyType || 'string',
        tableName: model.tableName,
        tableComment: model.tableComment,
        serviceStyle: serviceStyle.value,
        fields,
        ...headerCfg,
      };

      const codes = generateAll(config);
      CodePreviewPanel.render(context.extensionUri, codes);
    })
  );
}

function registerDatabaseCommand(context: vscode.ExtensionContext) {
  context.subscriptions.push(
    vscode.commands.registerCommand('kronos-codegen.generateFromDatabase', async () => {
      const { promptForDatabaseConfig } = require('./database/configManager') as typeof import('./database/configManager');
      const { testConnection, fetchTableList, fetchMultipleTableStructures, showOutput } = require('./database/connection') as typeof import('./database/connection');

      const dbConfig = await promptForDatabaseConfig();
      if (!dbConfig) { return; }

      try {
        await vscode.window.withProgress(
          { title: '正在连接数据库...', location: vscode.ProgressLocation.Notification },
          async () => {
            const connected = await testConnection(dbConfig);
            if (!connected) {
              vscode.window.showErrorMessage('数据库连接失败，请检查配置');
              return;
            }
          }
        );
      } catch {
        vscode.window.showErrorMessage('数据库连接失败，请检查配置');
        return;
      }

      let tableNames: string[];
      try {
        tableNames = await vscode.window.withProgress(
          { title: '正在获取表列表...', location: vscode.ProgressLocation.Notification },
          () => fetchTableList(dbConfig)
        );
      } catch (err) {
        vscode.window.showErrorMessage('获取表列表失败，请查看 Kronos CodeGen 输出面板了解详情');
        showOutput();
        return;
      }

      if (tableNames.length === 0) {
        vscode.window.showWarningMessage('数据库中没有找到表');
        return;
      }

      const selectedTables = await vscode.window.showQuickPick(
        tableNames.map(t => ({ label: t })),
        { placeHolder: '选择要生成代码的表（可多选）', canPickMany: true }
      );
      if (!selectedTables || selectedTables.length === 0) { return; }

      const tables = await vscode.window.withProgress(
        { title: '正在读取表结构...', location: vscode.ProgressLocation.Notification },
        () => fetchMultipleTableStructures(dbConfig, selectedTables.map(t => t.label))
      );

      const wsInfo = detectWorkspaceInfoFromWorkspace();

      const serviceStyle = await vscode.window.showQuickPick(
        [
          { label: '单 Service 结构体', value: 'single' as const, description: '所有方法挂在同一个 ServiceImpl 上' },
          { label: '独立 Service 结构体', value: 'separate' as const, description: '每个领域独立 Service' },
        ],
        { placeHolder: '选择 Service 风格' }
      );
      if (!serviceStyle) { return; }

      const moduleName = await vscode.window.showInputBox({
        prompt: 'Go Module 路径（自动检测，可修改）',
        value: wsInfo?.moduleName || '',
      });
      if (!moduleName) { return; }

      const pbPackage = await vscode.window.showInputBox({
        prompt: 'PB 包导入路径（自动检测，可修改）',
        value: wsInfo?.pbPackage || '',
      });
      if (!pbPackage) { return; }

      const pkgCfg = vscode.workspace.getConfiguration('kronos-codegen');
      const modelsPkgName = await vscode.window.showInputBox({
        prompt: 'Models 包名（目录名）',
        value: pkgCfg.get<string>('modelsPackageName', 'models'),
      }) || 'models';
      const repoPkgName = await vscode.window.showInputBox({
        prompt: 'Repository 包名（目录名）',
        value: pkgCfg.get<string>('repositoryPackageName', 'repository'),
      }) || 'repository';
      const servicePkgName = await vscode.window.showInputBox({
        prompt: 'Service 包名（目录名）',
        value: pkgCfg.get<string>('servicePackageName', 'service'),
      }) || 'service';
      const errorsPkgName = await vscode.window.showInputBox({
        prompt: 'Errors 包名（目录名）',
        value: pkgCfg.get<string>('errorsPackageName', 'errors'),
      }) || 'errors';

      const outputChoice = await vscode.window.showQuickPick(
        [
          { label: '预览代码', value: 'preview' as const, description: '在预览面板中查看和复制' },
          { label: '直接写入文件', value: 'write' as const, description: '自动写入到项目对应目录' },
        ],
        { placeHolder: '选择输出方式' }
      );

      let targetDir = '';
      if (outputChoice?.value === 'write') {
        const goModDirs = findGoModInWorkspace();
        if (goModDirs.length > 0) {
          const pick = await vscode.window.showQuickPick(
            goModDirs.map(d => ({ label: d, description: path.basename(d) })),
            { placeHolder: '选择目标项目（go.mod 所在目录）' }
          );
          if (pick) { targetDir = pick.label; }
        }
        if (!targetDir) {
          const picked = await vscode.window.showOpenDialog({
            canSelectFiles: false,
            canSelectFolders: true,
            canSelectMany: false,
            title: '选择目标项目目录',
          });
          if (picked && picked.length > 0) { targetDir = picked[0].fsPath; }
        }
        if (!targetDir) { return; }
      }

      const allCodes: import('./types').GeneratedCode[] = [];

      for (const table of tables) {
        const entityName = tableNameToEntityName(table.tableName);
        const fields = tableInfoToFieldConfigs(table);
        const pkField = fields.find(f => f.isPrimaryKey);
        const headerCfg = getFileHeaderConfig();

        const config: GeneratorConfig = {
          serviceName: `${entityName}Service`,
          moduleName,
          pbPackage,
          pbPackageAlias: wsInfo?.pbPackageAlias || 'pb',
          enumPackageAlias: wsInfo?.enumPackageAlias || 'enumspb',
          entityName,
          entityNameLower: entityName.charAt(0).toLowerCase() + entityName.slice(1),
          pbType: `${entityName}Info`,
          primaryKeyField: pkField?.goName || 'Id',
          primaryKeyType: pkField?.goType || 'string',
          tableName: table.tableName,
          tableComment: table.tableComment || `${entityName}表`,
          serviceStyle: serviceStyle.value,
          fields,
          ...headerCfg,
          modelsPackageName: modelsPkgName,
          repositoryPackageName: repoPkgName,
          servicePackageName: servicePkgName,
          errorsPackageName: errorsPkgName,
          modelsImportPath: `"github.com/${moduleName}/${modelsPkgName}"`,
          repositoryImportPath: `"github.com/${moduleName}/${repoPkgName}"`,
          serviceImportPath: `"github.com/${moduleName}/${servicePkgName}"`,
          errorsImportPath: `"github.com/${moduleName}/${errorsPkgName}"`,
          outputDir: targetDir,
        };

        const codes = generateAll(config);
        for (const c of codes) {
          allCodes.push({
            ...c,
            label: `[${entityName}] ${c.label}`,
          });
        }
      }

      if (outputChoice?.value === 'write' && targetDir) {
        const written = writeGeneratedFiles(targetDir, allCodes);
        vscode.window.showInformationMessage(`已写入 ${written} 个文件到 ${targetDir}`);
      } else {
        CodePreviewPanel.render(context.extensionUri, allCodes);
      }
    })
  );
}

function registerDatabaseProtoCommand(context: vscode.ExtensionContext) {
  context.subscriptions.push(
    vscode.commands.registerCommand('kronos-codegen.generateProtoFromDatabase', async () => {
      const { promptForDatabaseConfig } = require('./database/configManager') as typeof import('./database/configManager');
      const { testConnection, fetchTableList, fetchMultipleTableStructures, showOutput } = require('./database/connection') as typeof import('./database/connection');

      const dbConfig = await promptForDatabaseConfig();
      if (!dbConfig) { return; }

      try {
        await vscode.window.withProgress(
          { title: '正在连接数据库...', location: vscode.ProgressLocation.Notification },
          async () => {
            const connected = await testConnection(dbConfig);
            if (!connected) {
              vscode.window.showErrorMessage('数据库连接失败，请检查配置');
              return;
            }
          }
        );
      } catch {
        vscode.window.showErrorMessage('数据库连接失败，请检查配置');
        return;
      }

      let tableNames: string[];
      try {
        tableNames = await vscode.window.withProgress(
          { title: '正在获取表列表...', location: vscode.ProgressLocation.Notification },
          () => fetchTableList(dbConfig)
        );
      } catch (err) {
        vscode.window.showErrorMessage('获取表列表失败，请查看 Kronos CodeGen 输出面板了解详情');
        showOutput();
        return;
      }

      if (tableNames.length === 0) {
        vscode.window.showWarningMessage('数据库中没有找到表');
        return;
      }

      const selectedTables = await vscode.window.showQuickPick(
        tableNames.map(t => ({ label: t })),
        { placeHolder: '选择要生成 Proto 的表（可多选）', canPickMany: true }
      );
      if (!selectedTables || selectedTables.length === 0) { return; }

      const tables = await vscode.window.withProgress(
        { title: '正在读取表结构...', location: vscode.ProgressLocation.Notification },
        () => fetchMultipleTableStructures(dbConfig, selectedTables.map(t => t.label))
      );

      const packageName = await vscode.window.showInputBox({
        prompt: 'Proto package 名称',
        value: dbConfig.database.replace(/_/g, ''),
      });
      if (!packageName) { return; }

      const goPackage = await vscode.window.showInputBox({
        prompt: 'Go package 路径',
        value: `github.com/kamalyes/kronos-share-proto/pb/${packageName}`,
      });
      if (!goPackage) { return; }

      const serviceName = await vscode.window.showInputBox({
        prompt: 'Service 名称',
        value: `${tableNameToEntityName(tables[0].tableName)}Service`,
      });
      if (!serviceName) { return; }

      const includeCRUD = await vscode.window.showQuickPick(
        [
          { label: '是，生成完整 CRUD', value: true },
          { label: '否，仅生成 Message', value: false },
        ],
        { placeHolder: '是否生成 CRUD 请求/响应 Message 和 Service 定义？' }
      );

      const protoConfig: ProtoGeneratorConfig = {
        packageName,
        goPackage,
        javaPackage: `com.hitgame.proto.${packageName}`,
        tables,
        serviceName: serviceName || 'GeneratedService',
        includeCRUD: includeCRUD?.value ?? true,
      };

      const protoCode = generateProtoFromTables(protoConfig);

      const allCodes: import('./types').GeneratedCode[] = [protoCode];

      const shouldGenerateGo = await vscode.window.showQuickPick(
        [
          { label: '是，同时生成 Go 代码', value: true },
          { label: '否，仅生成 Proto', value: false },
        ],
        { placeHolder: '是否同时生成 Go Model/Repository/Service 代码？' }
      );

      if (shouldGenerateGo?.value) {
        const wsInfo = detectWorkspaceInfoFromWorkspace();

        const serviceStyle = await vscode.window.showQuickPick(
          [
            { label: '单 Service 结构体', value: 'single' as const },
            { label: '独立 Service 结构体', value: 'separate' as const },
          ],
          { placeHolder: '选择 Service 风格' }
        );

        const moduleName = await vscode.window.showInputBox({
          prompt: 'Go Module 路径',
          value: wsInfo?.moduleName || '',
        });
        if (!moduleName) { return; }

        const pbPackage = await vscode.window.showInputBox({
          prompt: 'PB 包导入路径',
          value: wsInfo?.pbPackage || goPackage || '',
        });
        if (!pbPackage) { return; }

        for (const table of tables) {
          const entityName = tableNameToEntityName(table.tableName);
          const fields = tableInfoToFieldConfigs(table);
          const pkField = fields.find(f => f.isPrimaryKey);
          const headerCfg = getFileHeaderConfig();

          const config: GeneratorConfig = {
            serviceName: serviceName || `${entityName}Service`,
            moduleName,
            pbPackage,
            pbPackageAlias: wsInfo?.pbPackageAlias || 'pb',
            enumPackageAlias: wsInfo?.enumPackageAlias || 'enumspb',
            entityName,
            entityNameLower: entityName.charAt(0).toLowerCase() + entityName.slice(1),
            pbType: `${entityName}Info`,
            primaryKeyField: pkField?.goName || 'Id',
            primaryKeyType: pkField?.goType || 'string',
            tableName: table.tableName,
            tableComment: table.tableComment || `${entityName}表`,
            serviceStyle: serviceStyle?.value || 'single',
            fields,
            ...headerCfg,
          };

          const codes = generateAll(config);
          for (const c of codes) {
            allCodes.push({
              ...c,
              label: `[${entityName}] ${c.label}`,
            });
          }
        }
      }

      CodePreviewPanel.render(context.extensionUri, allCodes);
    })
  );
}

function detectWorkspaceInfoFromWorkspace() {
  const workspaces = findGoModInWorkspace();
  if (workspaces.length === 0) { return null; }

  if (vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders.length > 0) {
    const currentPath = vscode.workspace.workspaceFolders[0].uri.fsPath;
    return detectWorkspaceInfo(currentPath);
  }

  return null;
}

function inferServiceName(services: any[], entityName: string): string {
  if (services.length > 0) {
    return services[0].name;
  }
  return `${entityName}Service`;
}

function inferPrimaryKey(msg: ProtoMessage): string {
  const firstField = msg.fields[0];
  if (firstField) {
    return firstField.name;
  }
  return 'id';
}

function inferTableName(entityName: string): string {
  const snake = entityName.replace(/([A-Z])/g, '_$1').toLowerCase().replace(/^_/, '');
  if (snake.endsWith('y')) {
    return snake.slice(0, -1) + 'ies';
  }
  return snake + 's';
}

function inferPrimaryKeyFromFields(fields: FieldConfig[]): string {
  const pk = fields.find(f => f.isPrimaryKey);
  if (pk) { return pk.goName; }
  const idField = fields.find(f => f.goName.endsWith('Id') || f.goName.endsWith('ID'));
  if (idField) { return idField.goName; }
  return 'Id';
}

function protoMessageToFieldConfigs(msg: ProtoMessage, parsed: any): FieldConfig[] {
  return msg.fields.map(f => {
    const goType = protoTypeToGoType(f.type, f.repeated);
    const isEnum = f.type.includes('enums.') || f.type.includes('enumspb.');
    const isCreatedAt = f.name === 'created_at';
    const isUpdatedAt = f.name === 'updated_at';
    const isI18N = f.name.includes('i18n') || f.type.includes('LocalizedText');

    return {
      protoName: f.name,
      goName: snakeToCamel(f.name),
      goType: isEnum ? `enumspb.${f.type.split('.').pop()}` : goType,
      gormTag: inferGormTag(snakeToCamel(f.name), f.type, f.number, f.repeated),
      jsonTag: f.name,
      comment: f.comment || f.name,
      isPrimaryKey: f.number === 1 && f.type === 'string',
      isEnum,
      enumType: isEnum ? f.type.split('.').pop() || '' : '',
      protoType: f.type,
      isUpdatedAt,
      isCreatedAt,
      isI18N,
      isOptionalWrapper: f.type.includes('StringValue') || f.type.includes('Int32Value'),
    };
  });
}

function snakeToCamel(str: string): string {
  return str.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
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

function showQuickPickWithDefault<T extends vscode.QuickPickItem>(
  items: T[],
  placeHolder: string
): Promise<T | undefined> {
  return new Promise(resolve => {
    const qp = vscode.window.createQuickPick<T>();
    qp.items = items;
    qp.placeholder = placeHolder;
    qp.activeItems = [items[0]];

    qp.onDidAccept(() => {
      const selected = qp.selectedItems[0] || qp.activeItems[0];
      qp.hide();
      resolve(selected);
    });

    qp.onDidHide(() => {
      resolve(undefined);
      qp.dispose();
    });

    qp.show();
  });
}

function getFileHeaderConfig(): { enableFileHeader: boolean; author: string; publisher: string } {
  const cfg = vscode.workspace.getConfiguration('kronos-codegen');
  const apiValue = cfg.get<boolean>('enableFileHeader', undefined as any);

  if (apiValue !== undefined) {
    return {
      enableFileHeader: cfg.get<boolean>('enableFileHeader', true),
      author: cfg.get<string>('author', 'kronos-codegen'),
      publisher: cfg.get<string>('publisher', 'kronos-team'),
    };
  }

  const fileCfg = readHeaderConfigFromFile();
  return {
    enableFileHeader: fileCfg.enableFileHeader ?? true,
    author: fileCfg.author ?? 'kronos-codegen',
    publisher: fileCfg.publisher ?? 'kronos-team',
  };
}

function readHeaderConfigFromFile(): { enableFileHeader?: boolean; author?: string; publisher?: string } {
  const fs = require('fs');
  const path = require('path');
  const workspaceFolders = vscode.workspace.workspaceFolders;
  if (!workspaceFolders || workspaceFolders.length === 0) { return {}; }

  const stripComments = (s: string) => s.replace(/\/\/.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');

  for (const folder of workspaceFolders) {
    for (const settingsDir of ['.trae', '.vscode']) {
      const settingsPath = path.join(folder.uri.fsPath, settingsDir, 'settings.json');
      if (fs.existsSync(settingsPath)) {
        try {
          const content = fs.readFileSync(settingsPath, 'utf-8');
          const json = JSON.parse(stripComments(content));
          return {
            enableFileHeader: json['kronos-codegen.enableFileHeader'],
            author: json['kronos-codegen.author'],
            publisher: json['kronos-codegen.publisher'],
          };
        } catch { /* ignore */ }
      }
    }
  }
  return {};
}

function writeGeneratedFiles(targetDir: string, codes: import('./types').GeneratedCode[]): number {
  let count = 0;
  for (const c of codes) {
    if (c.label.includes('Factory 注册')) { continue; }

    const filePath = path.join(targetDir, c.fileName);
    const dir = path.dirname(filePath);

    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }

    if (fs.existsSync(filePath)) {
      const overwrite = vscode.window.showWarningMessage(
        `文件 ${c.fileName} 已存在，是否覆盖？`,
        '覆盖', '跳过'
      );
    }

    fs.writeFileSync(filePath, c.code, 'utf-8');
    count++;
  }
  return count;
}
