import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';

type PromptAction = 'generateNewCode' | 'fieldChange' | 'implementInterface' | 'customTask';
type ModuleKind = 'service' | 'share-proto' | 'library' | 'unknown';

interface CommandDefinition {
  command: string;
  action: PromptAction;
}

interface ProjectInfo {
  root: string;
  moduleName: string;
  kind: ModuleKind;
  presentDirs: string[];
  relevantDependencies: string[];
}

interface SelectionSummary {
  sourceKind: string;
  entityName: string;
  symbols: string[];
  rpcMethods: string[];
  fields: string[];
  searchTerms: string[];
}

interface PromptContext {
  action: PromptAction;
  customGoal?: string;
  target: ProjectInfo;
  source: {
    filePath: string;
    relativePath: string;
    languageId: string;
    lineStart: number;
    lineEnd: number;
  };
  selection: string;
  summary: SelectionSummary;
  workspaceModules: ProjectInfo[];
}

const commands: CommandDefinition[] = [
  { command: 'kronos-codegen.generateNewCodePrompt', action: 'generateNewCode' },
  { command: 'kronos-codegen.fieldChangePrompt', action: 'fieldChange' },
  { command: 'kronos-codegen.implementInterfacePrompt', action: 'implementInterface' },
  { command: 'kronos-codegen.customTaskPrompt', action: 'customTask' },
];

const importantDirs = [
  'models',
  'repository',
  'service',
  'errors',
  'bootstrap',
  'migrations',
  'proto',
  'protos',
  'internal',
  'constants',
];

const dependencyHints = [
  'apex-share-proto',
  'go-pbmo',
  'go-sqlbuilder',
  'go-rpc-gateway',
  'grpc-gateway',
  'gorm.io/gorm',
];

export function activate(context: vscode.ExtensionContext) {
  for (const item of commands) {
    context.subscriptions.push(
      vscode.commands.registerCommand(item.command, () => runPromptAction(item.action))
    );
  }
}

export function deactivate() {
  // No background resources are held by the simplified prompt-only extension.
}

async function runPromptAction(action: PromptAction) {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    vscode.window.showWarningMessage('Kronos: 请先打开并选中一段 Proto 或 Go 代码。');
    return;
  }

  if (!isSupportedDocument(editor.document)) {
    vscode.window.showWarningMessage('Kronos: 当前只处理 .proto 和 .go 选区。');
    return;
  }

  const selection = editor.document.getText(editor.selection).trim();
  if (!selection) {
    vscode.window.showWarningMessage('Kronos: 请先选中要交给 Codex 分析的 proto、model、接口或方法片段。');
    return;
  }

  const customGoal = action === 'customTask'
    ? await vscode.window.showInputBox({
      prompt: '补充你希望 Codex 完成的具体任务',
      placeHolder: '例如：补齐批量导入接口、改造查询条件、同步新增字段影响面',
      ignoreFocusOut: true,
    })
    : undefined;
  if (action === 'customTask' && !customGoal) {
    return;
  }

  const targetRoot = await resolveTargetRoot(editor.document.uri);
  if (!targetRoot) {
    vscode.window.showWarningMessage('Kronos: 没有找到可用的目标 Go 服务目录。');
    return;
  }

  const context = await buildPromptContext(editor, selection, action, targetRoot, customGoal);
  const prompt = buildCodexPrompt(context);
  await sendPromptToCodex(prompt);
}

async function buildPromptContext(
  editor: vscode.TextEditor,
  selection: string,
  action: PromptAction,
  targetRoot: string,
  customGoal?: string
): Promise<PromptContext> {
  const document = editor.document;
  const target = readProjectInfo(targetRoot);
  const workspaceModules = collectGoModuleRoots()
    .map(readProjectInfo)
    .sort((a, b) => kindRank(a.kind) - kindRank(b.kind) || a.root.localeCompare(b.root));

  return {
    action,
    customGoal,
    target,
    source: {
      filePath: document.uri.fsPath,
      relativePath: relativeToRoot(target.root, document.uri.fsPath),
      languageId: document.languageId,
      lineStart: editor.selection.start.line + 1,
      lineEnd: editor.selection.end.line + 1,
    },
    selection,
    summary: summarizeSelection(selection, document.languageId),
    workspaceModules,
  };
}

async function resolveTargetRoot(uri: vscode.Uri): Promise<string | undefined> {
  const activePath = uri.fsPath;
  const nearestModuleRoot = findNearestGoModRoot(activePath);
  if (nearestModuleRoot && classifyModuleRoot(nearestModuleRoot) === 'service') {
    return nearestModuleRoot;
  }

  const serviceRoots = collectGoModuleRoots().filter(root => classifyModuleRoot(root) === 'service');
  if (serviceRoots.length === 1) {
    return serviceRoots[0];
  }

  if (serviceRoots.length > 1) {
    const picked = await vscode.window.showQuickPick(
      serviceRoots.map(root => {
        const info = readProjectInfo(root);
        return {
          label: path.basename(root),
          description: info.moduleName || root,
          detail: root,
          root,
        };
      }),
      {
        placeHolder: '选择 Codex 要写入的目标服务。选区可以来自 share-proto，但实现代码应落到具体服务里。',
        ignoreFocusOut: true,
      }
    );
    return picked?.root;
  }

  return nearestModuleRoot || vscode.workspace.getWorkspaceFolder(uri)?.uri.fsPath;
}

function buildCodexPrompt(ctx: PromptContext): string {
  const fence = fenceCode(ctx.selection, languageFence(ctx.source.languageId, ctx.source.filePath));
  const task = actionInstruction(ctx.action, ctx.customGoal);
  const searchTerms = ctx.summary.searchTerms.length > 0
    ? ctx.summary.searchTerms.map(term => `\`${term}\``).join(', ')
    : '(Codex 先根据选区自行提取实体名、接口名、RPC 名)';
  const modules = ctx.workspaceModules.length > 0
    ? ctx.workspaceModules.map(formatModuleInfo).join('\n')
    : '- 未扫描到其他 go.mod 模块';
  const dependencies = ctx.target.relevantDependencies.length > 0
    ? ctx.target.relevantDependencies.map(line => `- ${line}`).join('\n')
    : '- 未在目标 go.mod 中识别到 Apex 相关依赖，请以真实项目为准';
  const dirs = ctx.target.presentDirs.length > 0 ? ctx.target.presentDirs.join(', ') : '(未识别到常见目录)';

  return `你是 Codex，请直接在本机目标 Go 服务仓库里完成代码实现。

# 任务
${task}

# 目标服务
- 根目录: ${ctx.target.root}
- Go module: ${ctx.target.moduleName || '(未识别，请读取 go.mod 确认)'}
- 类型判断: ${ctx.target.kind}
- 已有常见目录: ${dirs}
- 相关依赖:
${dependencies}

# 选区来源
- 文件: ${ctx.source.filePath}
- 相对目标服务路径: ${ctx.source.relativePath}
- 语言: ${ctx.source.languageId}
- 行号: ${ctx.source.lineStart}-${ctx.source.lineEnd}
- 选区类型判断: ${ctx.summary.sourceKind}
- 识别到的实体/主题: ${ctx.summary.entityName || '(未识别)'}
- 识别到的符号: ${ctx.summary.symbols.length > 0 ? ctx.summary.symbols.join(', ') : '(无)'}
- 识别到的 RPC: ${ctx.summary.rpcMethods.length > 0 ? ctx.summary.rpcMethods.join(', ') : '(无)'}
- 识别到的字段: ${ctx.summary.fields.length > 0 ? ctx.summary.fields.join(', ') : '(无)'}
- 建议优先搜索: ${searchTerms}

# 选中的代码
${fence}

# 工作区模块线索
${modules}

# Apex 风格实现约束
- Kronos 扩展只负责整理上下文和提示词，不负责生成最终文件；最终代码必须由你读取目标仓库后落地。
- 如果选区来自 \`apex-share-proto\` 或其他共享 proto 仓库，只把它当接口契约输入，不要把共享 proto 仓库当成服务实现目录。
- 先在目标服务根目录运行 \`rg\` 搜索相近实体、RPC、Model、Repository、Service、PBMO、错误码和注册方式，再按真实写法实现。
- 常见 Apex 服务目录是 \`models\`、\`repository\`、\`service\`、\`errors\`、\`bootstrap\`、\`migrations\`、\`constants\`，但以当前服务真实结构为准。
- Model 层优先保持现有 GORM tag、json tag、TableName、TableComment、字段命名和注释风格。
- Repository 层优先复用 \`github.com/kamalyes/go-sqlbuilder/repository\` 的 BaseRepository、Query、Filter、Update API。
- Service 层优先复用当前服务的 receiver、错误处理、分页、上下文取 tenant/user/platform/region 的方式。
- PB/Model 转换优先复用 \`github.com/kamalyes/go-pbmo\` 的 Register、ToPB、ToPBs、FromPB、NewUpdates 等现有项目写法。
- 错误码放到现有 \`errors\` 包的合适位置，并同步 gRPC 映射、i18n 或测试，仅在目标项目已有这些机制时添加。
- 新增实体时检查是否需要 repository factory、bootstrap/pbmo 注册、服务注册、迁移、常量、测试或 README 片段。
- 字段变更时同步 model、proto request/response、service 校验、repository 查询/更新、PBMO 映射、迁移和测试中的影响面。
- 实现接口时如果选区是 proto service/rpc，请补齐对应服务方法、注册/embedding、依赖注入和必要的 repository 调用；如果选区是 Go interface，请找当前服务已有实现类型并补齐方法。
- 不要凭空发明目录结构。没有把握时先列出你搜索到的参考文件，再按参考文件风格修改。
- 完成后运行最小必要的格式化和验证命令，例如 \`gofmt\`、相关包 \`go test\`，必要时再运行 \`go test ./...\`。

# 输出要求
- 直接修改目标服务代码。
- 汇报参考了哪些文件、改了哪些文件、验证命令结果。
- 如果某个影响面因为缺少上下文不能安全处理，说明原因并给出下一步最小动作。`;
}

function actionInstruction(action: PromptAction, customGoal?: string): string {
  switch (action) {
    case 'generateNewCode':
      return '根据选中的 Proto / Go Model / 接口片段，在目标服务中补齐新代码。重点是让 Codex 读取真实项目后生成缺失的 model、repository、service、错误码、PBMO 注册、factory/bootstrap wiring 等必要代码，而不是让扩展内置模板直接写文件。';
    case 'fieldChange':
      return '选区代表字段结构的新目标形态。请把同名实体或接口的字段变更同步到目标服务所有受影响代码，包括 model、proto request/response、PBMO、repository 查询/更新、service 校验/组装、错误码、迁移和测试。';
    case 'implementInterface':
      return '根据选中的 proto rpc、proto service、Go interface 或方法签名，在目标服务中实现这个接口及其调用链。优先复用当前 Apex 服务已有的 receiver、注册、错误处理、repository 和 PBMO 写法。';
    case 'customTask':
      return customGoal || '根据选区完成用户指定的自定义代码生成或改造任务。';
  }
}

async function sendPromptToCodex(prompt: string) {
  await vscode.env.clipboard.writeText(prompt);

  const command = vscode.workspace
    .getConfiguration('kronos-codegen')
    .get<string>('codexCommand', 'workbench.action.chat.open')
    .trim();

  if (!command) {
    vscode.window.showInformationMessage('Kronos: Codex 提示词已复制到剪贴板。');
    return;
  }

  try {
    await vscode.commands.executeCommand(command, prompt);
    vscode.window.showInformationMessage('Kronos: Codex 提示词已复制，并已尝试打开 Codex/Chat。');
  } catch {
    try {
      await vscode.commands.executeCommand(command);
      vscode.window.showInformationMessage('Kronos: Codex 提示词已复制，已打开配置的 Codex/Chat 命令。');
    } catch {
      vscode.window.showInformationMessage('Kronos: Codex 提示词已复制到剪贴板，未找到可打开的 Codex/Chat 命令。');
    }
  }
}

function summarizeSelection(text: string, languageId: string): SelectionSummary {
  const protoMessages = [...text.matchAll(/\bmessage\s+([A-Za-z_]\w*)/g)].map(match => match[1]);
  const protoServices = [...text.matchAll(/\bservice\s+([A-Za-z_]\w*)/g)].map(match => match[1]);
  const protoEnums = [...text.matchAll(/\benum\s+([A-Za-z_]\w*)/g)].map(match => match[1]);
  const rpcMethods = [...text.matchAll(/\brpc\s+([A-Za-z_]\w*)\s*\(/g)].map(match => match[1]);
  const goStructs = [...text.matchAll(/\btype\s+([A-Za-z_]\w*)\s+struct\b/g)].map(match => match[1]);
  const goInterfaces = [...text.matchAll(/\btype\s+([A-Za-z_]\w*)\s+interface\b/g)].map(match => match[1]);
  const goFuncs = [...text.matchAll(/\bfunc\s+(?:\([^)]+\)\s*)?([A-Za-z_]\w*)\s*\(/g)].map(match => match[1]);
  const symbols = unique([...protoMessages, ...protoServices, ...protoEnums, ...goStructs, ...goInterfaces, ...goFuncs]);
  const fields = unique([...extractGoFields(text), ...extractProtoFields(text)]).slice(0, 40);
  const entityName = inferEntityName([...goStructs, ...protoMessages, ...goInterfaces, ...protoServices, ...rpcMethods]);
  const sourceKind = inferSourceKind(languageId, {
    protoMessages,
    protoServices,
    rpcMethods,
    goStructs,
    goInterfaces,
    goFuncs,
  });
  const searchTerms = buildSearchTerms(entityName, symbols, rpcMethods);

  return {
    sourceKind,
    entityName,
    symbols,
    rpcMethods,
    fields,
    searchTerms,
  };
}

function extractGoFields(text: string): string[] {
  return [...text.matchAll(/^\s*([A-Z][A-Za-z0-9_]*)\s+([*\[\]A-Za-z0-9_.]+)(?:\s+`([^`]+)`)?.*$/gm)]
    .map(match => {
      const tag = match[3] ? ` ${match[3]}` : '';
      return `${match[1]}: ${match[2]}${tag}`;
    });
}

function extractProtoFields(text: string): string[] {
  return [...text.matchAll(/^\s*(?:optional\s+|repeated\s+)?([A-Za-z_][\w.]*)\s+([A-Za-z_]\w*)\s*=\s*\d+/gm)]
    .map(match => `${match[2]}: ${match[1]}`);
}

function inferEntityName(names: string[]): string {
  const first = names.find(Boolean) || '';
  return first.replace(/(Model|Info|Request|Response|Service|Server|Repository)$/g, '');
}

function inferSourceKind(languageId: string, parts: {
  protoMessages: string[];
  protoServices: string[];
  rpcMethods: string[];
  goStructs: string[];
  goInterfaces: string[];
  goFuncs: string[];
}): string {
  if (parts.rpcMethods.length > 0) {
    return 'proto rpc';
  }
  if (parts.protoServices.length > 0) {
    return 'proto service';
  }
  if (parts.protoMessages.length > 0) {
    return 'proto message';
  }
  if (parts.goInterfaces.length > 0) {
    return 'go interface';
  }
  if (parts.goStructs.length > 0) {
    return 'go struct/model';
  }
  if (parts.goFuncs.length > 0) {
    return 'go function/method';
  }
  return languageId || 'unknown';
}

function buildSearchTerms(entityName: string, symbols: string[], rpcMethods: string[]): string[] {
  const entityTerms = entityName
    ? [
      entityName,
      `${entityName}Model`,
      `${entityName}Info`,
      `${entityName}Repository`,
      `${entityName}Create`,
      `${entityName}Update`,
      `${entityName}List`,
      `BizErrCode${entityName}`,
    ]
    : [];
  return unique([...entityTerms, ...symbols, ...rpcMethods]).filter(Boolean).slice(0, 16);
}

function readProjectInfo(root: string): ProjectInfo {
  return {
    root,
    moduleName: readGoModuleName(root),
    kind: classifyModuleRoot(root),
    presentDirs: importantDirs.filter(dir => fs.existsSync(path.join(root, dir))),
    relevantDependencies: readRelevantDependencies(root),
  };
}

function collectGoModuleRoots(): string[] {
  const roots = new Set<string>();
  for (const folder of vscode.workspace.workspaceFolders || []) {
    scanForGoModules(folder.uri.fsPath, 2, roots);
  }
  return [...roots];
}

function scanForGoModules(dir: string, depth: number, roots: Set<string>) {
  if (fs.existsSync(path.join(dir, 'go.mod'))) {
    roots.add(dir);
  }
  if (depth <= 0) {
    return;
  }

  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }

  for (const entry of entries) {
    if (!entry.isDirectory() || shouldSkipDir(entry.name)) {
      continue;
    }
    scanForGoModules(path.join(dir, entry.name), depth - 1, roots);
  }
}

function findNearestGoModRoot(filePath: string): string | undefined {
  let dir = fs.existsSync(filePath) && fs.statSync(filePath).isDirectory()
    ? filePath
    : path.dirname(filePath);

  while (dir && dir !== path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, 'go.mod'))) {
      return dir;
    }
    dir = path.dirname(dir);
  }
  return undefined;
}

function classifyModuleRoot(root: string): ModuleKind {
  const base = path.basename(root).toLowerCase();
  const moduleName = readGoModuleName(root).toLowerCase();
  const hasServiceDirs = fs.existsSync(path.join(root, 'service')) &&
    fs.existsSync(path.join(root, 'repository')) &&
    fs.existsSync(path.join(root, 'models'));

  if (base.includes('share-proto') || moduleName.includes('share-proto')) {
    return 'share-proto';
  }
  if (base.endsWith('-service') || hasServiceDirs) {
    return 'service';
  }
  if (base.startsWith('go-') || base.includes('gateway') || base.includes('sqlbuilder') || base.includes('pbmo')) {
    return 'library';
  }
  return 'unknown';
}

function readGoModuleName(root: string): string {
  const goMod = path.join(root, 'go.mod');
  if (!fs.existsSync(goMod)) {
    return '';
  }
  const content = fs.readFileSync(goMod, 'utf-8');
  return content.match(/^module\s+(\S+)/m)?.[1] || '';
}

function readRelevantDependencies(root: string): string[] {
  const goMod = path.join(root, 'go.mod');
  if (!fs.existsSync(goMod)) {
    return [];
  }
  return fs.readFileSync(goMod, 'utf-8')
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(line => dependencyHints.some(dep => line.includes(dep)))
    .slice(0, 24);
}

function kindRank(kind: ModuleKind): number {
  if (kind === 'service') {
    return 0;
  }
  if (kind === 'share-proto') {
    return 1;
  }
  if (kind === 'library') {
    return 2;
  }
  return 3;
}

function isSupportedDocument(document: vscode.TextDocument): boolean {
  return document.languageId === 'go' ||
    document.languageId === 'proto' ||
    document.fileName.endsWith('.go') ||
    document.fileName.endsWith('.proto');
}

function languageFence(languageId: string, filePath: string): string {
  if (languageId === 'go' || filePath.endsWith('.go')) {
    return 'go';
  }
  if (languageId === 'proto' || filePath.endsWith('.proto')) {
    return 'proto';
  }
  return 'text';
}

function fenceCode(code: string, language: string): string {
  return `\`\`\`${language}\n${code.replace(/```/g, '`\\`\\`')}\n\`\`\``;
}

function formatModuleInfo(info: ProjectInfo): string {
  const moduleName = info.moduleName || '(unknown module)';
  return `- ${info.kind}: ${info.root} (${moduleName})`;
}

function relativeToRoot(root: string, filePath: string): string {
  const relative = path.relative(root, filePath);
  if (!relative || relative.startsWith('..')) {
    return filePath;
  }
  return relative.replace(/\\/g, '/');
}

function shouldSkipDir(name: string): boolean {
  return [
    '.git',
    '.vscode',
    'node_modules',
    'vendor',
    'out',
    'dist',
    'build',
    'coverage',
  ].includes(name);
}

function unique(values: string[]): string[] {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))];
}
