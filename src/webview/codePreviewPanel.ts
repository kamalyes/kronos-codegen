import * as vscode from 'vscode';
import { GeneratedCode } from '../types';

export class CodePreviewPanel {
  public static currentPanel: CodePreviewPanel | undefined;
  private readonly _panel: vscode.WebviewPanel;
  private _disposables: vscode.Disposable[] = [];

  private constructor(panel: vscode.WebviewPanel, extensionUri: vscode.Uri) {
    this._panel = panel;
    this._panel.onDidDispose(() => this.dispose(), null, this._disposables);
  }

  public static render(extensionUri: vscode.Uri, codes: GeneratedCode[]) {
    if (CodePreviewPanel.currentPanel) {
      CodePreviewPanel.currentPanel._panel.reveal(vscode.ViewColumn.Beside);
      CodePreviewPanel.currentPanel._update(codes);
      return;
    }

    const panel = vscode.window.createWebviewPanel(
      'kronosCodePreview',
      'Kronos Code Generator',
      vscode.ViewColumn.Beside,
      { enableScripts: true }
    );

    CodePreviewPanel.currentPanel = new CodePreviewPanel(panel, extensionUri);
    CodePreviewPanel.currentPanel._update(codes);
  }

  private _update(codes: GeneratedCode[]) {
    this._panel.webview.html = this._getHtmlForWebview(codes);

    this._panel.webview.onDidReceiveMessage(
      async (message) => {
        if (message.command === 'copy') {
          await vscode.env.clipboard.writeText(message.code);
          vscode.window.showInformationMessage(`已复制 ${message.label} 代码到剪贴板`);
        } else if (message.command === 'copyAll') {
          await vscode.env.clipboard.writeText(message.code);
          vscode.window.showInformationMessage(`已复制全部 ${message.count} 个文件代码到剪贴板`);
        }
      },
      null,
      this._disposables
    );
  }

  private _getHtmlForWebview(codes: GeneratedCode[]): string {
    const isMultiEntity = codes.some(c => c.label.startsWith('['));

    if (isMultiEntity) {
      return this._getMultiEntityHtml(codes);
    }
    return this._getSingleEntityHtml(codes);
  }

  private _getSingleEntityHtml(codes: GeneratedCode[]): string {
    const tabs = codes.map((c, i) =>
      `<button class="tab-btn ${i === 0 ? 'active' : ''}" onclick="switchTab(${i})">${c.label}</button>`
    ).join('');

    const panels = codes.map((c, i) => `
      <div class="tab-panel ${i === 0 ? 'active' : ''}" id="panel-${i}">
        <div class="panel-header">
          <span class="file-name">${c.fileName}</span>
          <button class="copy-btn" onclick="copyCode(${i})">📋 复制代码</button>
        </div>
        <pre><code>${escapeHtml(c.code)}</code></pre>
      </div>
    `).join('');

    const codeData = codes.map((c, i) =>
      `const code_${i} = ${JSON.stringify(c.code)}; const label_${i} = ${JSON.stringify(c.label)};`
    ).join('\n');

    return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>Kronos Code Generator</title>
  <style>${commonStyles()}</style>
</head>
<body>
  <div class="header">
    <h1>🚀 Kronos Code Generator</h1>
    <span class="badge">${codes.length} 个文件</span>
  </div>
  <div class="tabs">${tabs}</div>
  ${panels}
  <script>
    ${codeData}
    function switchTab(index) {
      document.querySelectorAll('.tab-btn').forEach((btn, i) => btn.classList.toggle('active', i === index));
      document.querySelectorAll('.tab-panel').forEach((panel, i) => panel.classList.toggle('active', i === index));
    }
    function copyCode(index) {
      const vscode = acquireVsCodeApi();
      vscode.postMessage({ command: 'copy', code: eval('code_' + index), label: eval('label_' + index) });
    }
  </script>
</body>
</html>`;
  }

  private _getMultiEntityHtml(codes: GeneratedCode[]): string {
    const groups = new Map<string, { index: number; code: GeneratedCode }[]>();
    codes.forEach((c, i) => {
      const match = c.label.match(/^\[(.+?)\]\s*(.*)/);
      const entityName = match ? match[1] : 'Other';
      const subLabel = match ? match[2] : c.label;
      if (!groups.has(entityName)) { groups.set(entityName, []); }
      groups.get(entityName)!.push({ index: i, code: { ...c, label: subLabel } });
    });

    const entityNames = Array.from(groups.keys());

    const entityTabs = entityNames.map((name, i) =>
      `<button class="entity-tab ${i === 0 ? 'active' : ''}" onclick="switchEntity(${i})">${name}</button>`
    ).join('');

    const entityPanels = entityNames.map((name, ei) => {
      const items = groups.get(name)!;
      const subTabs = items.map((item, si) =>
        `<button class="sub-tab ${si === 0 ? 'active' : ''}" onclick="switchSubTab(${ei}, ${si})">${item.code.label}</button>`
      ).join('');

      const subPanels = items.map((item, si) => `
        <div class="sub-panel ${si === 0 ? 'active' : ''}" id="sub-${ei}-${si}">
          <div class="panel-header">
            <span class="file-name">${item.code.fileName}</span>
            <button class="copy-btn" onclick="copyCode(${item.index})">📋 复制代码</button>
          </div>
          <pre><code>${escapeHtml(item.code.code)}</code></pre>
        </div>
      `).join('');

      return `
        <div class="entity-panel ${ei === 0 ? 'active' : ''}" id="entity-${ei}">
          <div class="sub-tabs">${subTabs}</div>
          ${subPanels}
        </div>
      `;
    }).join('');

    const codeData = codes.map((c, i) =>
      `const code_${i} = ${JSON.stringify(c.code)}; const label_${i} = ${JSON.stringify(c.label)};`
    ).join('\n');

    const allCode = codes.map(c => `// --- ${c.fileName} ---\n${c.code}`).join('\n\n');

    return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <title>Kronos Code Generator</title>
  <style>
    ${commonStyles()}
    .entity-tabs {
      display: flex;
      gap: 2px;
      margin-bottom: 0;
      flex-wrap: wrap;
      padding: 8px 0 0 0;
    }
    .entity-tab {
      padding: 6px 14px;
      border: 1px solid var(--vscode-panel-border);
      border-bottom: none;
      border-radius: 6px 6px 0 0;
      background: var(--vscode-tab-inactiveBackground);
      color: var(--vscode-tab-inactiveForeground);
      cursor: pointer;
      font-size: 12px;
      transition: all 0.15s;
    }
    .entity-tab:hover {
      background: var(--vscode-tab-activeBackground);
    }
    .entity-tab.active {
      background: var(--vscode-tab-activeBackground);
      color: var(--vscode-tab-activeForeground);
      border-bottom: 2px solid var(--vscode-focusBorder);
      font-weight: 600;
    }
    .entity-panel {
      display: none;
      border: 1px solid var(--vscode-panel-border);
      border-radius: 0 6px 6px 6px;
      overflow: hidden;
    }
    .entity-panel.active {
      display: block;
    }
    .sub-tabs {
      display: flex;
      gap: 2px;
      padding: 8px 8px 0 8px;
      background: var(--vscode-editorGroupHeader-tabsBackground);
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    .sub-tab {
      padding: 4px 12px;
      border: 1px solid transparent;
      border-bottom: none;
      border-radius: 4px 4px 0 0;
      background: transparent;
      color: var(--vscode-tab-inactiveForeground);
      cursor: pointer;
      font-size: 12px;
      transition: all 0.15s;
    }
    .sub-tab:hover {
      color: var(--vscode-tab-activeForeground);
    }
    .sub-tab.active {
      background: var(--vscode-editor-background);
      color: var(--vscode-tab-activeForeground);
      border-color: var(--vscode-panel-border);
      border-bottom: 1px solid var(--vscode-editor-background);
      font-weight: 500;
      margin-bottom: -1px;
    }
    .sub-panel {
      display: none;
    }
    .sub-panel.active {
      display: block;
    }
    .copy-all-btn {
      padding: 4px 12px;
      border: 1px solid var(--vscode-button-border);
      border-radius: 4px;
      background: var(--vscode-button-secondaryBackground);
      color: var(--vscode-button-secondaryForeground);
      cursor: pointer;
      font-size: 12px;
    }
    .copy-all-btn:hover {
      opacity: 0.9;
    }
  </style>
</head>
<body>
  <div class="header">
    <h1>🚀 Kronos Code Generator</h1>
    <span class="badge">${entityNames.length} 个实体 · ${codes.length} 个文件</span>
    <button class="copy-all-btn" onclick="copyAll()">📋 复制全部</button>
  </div>
  <div class="entity-tabs">${entityTabs}</div>
  ${entityPanels}
  <script>
    ${codeData}
    const allCode = ${JSON.stringify(allCode)};

    function switchEntity(index) {
      document.querySelectorAll('.entity-tab').forEach((btn, i) => btn.classList.toggle('active', i === index));
      document.querySelectorAll('.entity-panel').forEach((panel, i) => panel.classList.toggle('active', i === index));
    }

    function switchSubTab(entityIndex, subIndex) {
      const panel = document.getElementById('entity-' + entityIndex);
      panel.querySelectorAll('.sub-tab').forEach((btn, i) => btn.classList.toggle('active', i === subIndex));
      panel.querySelectorAll('.sub-panel').forEach((p, i) => p.classList.toggle('active', i === subIndex));
    }

    function copyCode(index) {
      const vscode = acquireVsCodeApi();
      vscode.postMessage({ command: 'copy', code: eval('code_' + index), label: eval('label_' + index) });
    }

    function copyAll() {
      const vscode = acquireVsCodeApi();
      vscode.postMessage({ command: 'copyAll', code: allCode, count: ${codes.length} });
    }
  </script>
</body>
</html>`;
  }

  public dispose() {
    CodePreviewPanel.currentPanel = undefined;
    this._panel.dispose();
    while (this._disposables.length) {
      const x = this._disposables.pop();
      if (x) { x.dispose(); }
    }
  }
}

function commonStyles(): string {
  return `
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: var(--vscode-editor-background);
      color: var(--vscode-editor-foreground);
      padding: 16px;
    }
    .header {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 16px;
      padding-bottom: 12px;
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    .header h1 { font-size: 18px; font-weight: 600; }
    .badge {
      padding: 2px 8px;
      border-radius: 10px;
      font-size: 11px;
      background: var(--vscode-button-background);
      color: var(--vscode-button-foreground);
    }
    .tabs {
      display: flex;
      gap: 4px;
      margin-bottom: 0;
      flex-wrap: wrap;
    }
    .tab-btn {
      padding: 8px 16px;
      border: 1px solid var(--vscode-panel-border);
      border-bottom: none;
      border-radius: 6px 6px 0 0;
      background: var(--vscode-tab-inactiveBackground);
      color: var(--vscode-tab-inactiveForeground);
      cursor: pointer;
      font-size: 13px;
      transition: all 0.15s;
    }
    .tab-btn:hover { background: var(--vscode-tab-activeBackground); }
    .tab-btn.active {
      background: var(--vscode-tab-activeBackground);
      color: var(--vscode-tab-activeForeground);
      border-bottom: 2px solid var(--vscode-focusBorder);
      font-weight: 600;
    }
    .tab-panel {
      display: none;
      border: 1px solid var(--vscode-panel-border);
      border-radius: 0 6px 6px 6px;
      overflow: hidden;
    }
    .tab-panel.active { display: block; }
    .panel-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 8px 16px;
      background: var(--vscode-editorGroupHeader-tabsBackground);
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    .file-name {
      font-family: monospace;
      font-size: 12px;
      color: var(--vscode-descriptionForeground);
    }
    .copy-btn {
      padding: 4px 12px;
      border: 1px solid var(--vscode-button-border);
      border-radius: 4px;
      background: var(--vscode-button-background);
      color: var(--vscode-button-foreground);
      cursor: pointer;
      font-size: 12px;
      transition: opacity 0.15s;
    }
    .copy-btn:hover { opacity: 0.9; }
    .copy-btn:active { opacity: 0.8; }
    pre {
      margin: 0;
      padding: 16px;
      overflow-x: auto;
      font-size: 13px;
      line-height: 1.5;
      font-family: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', Consolas, monospace;
    }
    code { white-space: pre; }
  `;
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
