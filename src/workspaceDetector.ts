import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';

export interface WorkspaceInfo {
  moduleName: string;
  modulePath: string;
  pbPackage: string;
  pbPackageAlias: string;
  enumPackage: string;
  enumPackageAlias: string;
  protoPackage: string;
  goPackage: string;
}

export function detectWorkspaceInfo(filePath: string): WorkspaceInfo | null {
  const workspaceRoot = findWorkspaceRoot(filePath);
  if (!workspaceRoot) {
    return null;
  }

  const moduleName = extractModuleName(workspaceRoot);
  const pbInfo = extractPBPackageInfo(workspaceRoot, filePath);

  return {
    moduleName: moduleName || '',
    modulePath: workspaceRoot,
    pbPackage: pbInfo.pbPackage || '',
    pbPackageAlias: pbInfo.pbPackageAlias || 'pb',
    enumPackage: pbInfo.enumPackage || '',
    enumPackageAlias: pbInfo.enumPackageAlias || 'enumspb',
    protoPackage: pbInfo.protoPackage || '',
    goPackage: pbInfo.goPackage || '',
  };
}

function findWorkspaceRoot(filePath: string): string | null {
  let dir = path.dirname(filePath);

  while (dir !== path.dirname(dir)) {
    if (fs.existsSync(path.join(dir, 'go.mod'))) {
      return dir;
    }
    dir = path.dirname(dir);
  }

  if (vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders.length > 0) {
    return vscode.workspace.workspaceFolders[0].uri.fsPath;
  }

  return null;
}

function extractModuleName(workspaceRoot: string): string | null {
  const goModPath = path.join(workspaceRoot, 'go.mod');
  if (!fs.existsSync(goModPath)) {
    return null;
  }

  const content = fs.readFileSync(goModPath, 'utf-8');
  const match = content.match(/^module\s+([\S]+)/m);
  return match ? match[1] : null;
}

interface PBPackageInfo {
  pbPackage: string;
  pbPackageAlias: string;
  enumPackage: string;
  enumPackageAlias: string;
  protoPackage: string;
  goPackage: string;
}

function extractPBPackageInfo(workspaceRoot: string, filePath: string): PBPackageInfo {
  const result: PBPackageInfo = {
    pbPackage: '',
    pbPackageAlias: 'pb',
    enumPackage: '',
    enumPackageAlias: 'enumspb',
    protoPackage: '',
    goPackage: '',
  };

  const modelsDir = findModelsDir(workspaceRoot, filePath);
  if (modelsDir && fs.existsSync(modelsDir)) {
    const files = fs.readdirSync(modelsDir).filter(f => f.endsWith('_model.go'));
    for (const file of files) {
      const content = fs.readFileSync(path.join(modelsDir, file), 'utf-8');
      const pbMatch = content.match(/(\w+)\s+"([^"]*share-proto[^"]*\/pb\/[^"]+)"/);
      if (pbMatch) {
        result.pbPackageAlias = pbMatch[1];
        result.pbPackage = pbMatch[2];
        const parts = pbMatch[2].split('/');
        result.protoPackage = parts[parts.length - 1] || '';
        break;
      }
    }

    for (const file of files) {
      const content = fs.readFileSync(path.join(modelsDir, file), 'utf-8');
      const enumMatch = content.match(/(\w+)\s+"([^"]*share-proto[^"]*\/pb\/enums)"/);
      if (enumMatch) {
        result.enumPackageAlias = enumMatch[1];
        result.enumPackage = enumMatch[2];
        break;
      }
    }
  }

  const protoDir = findProtoDir(workspaceRoot);
  if (protoDir) {
    const protoFiles = listProtoFiles(protoDir);
    for (const pf of protoFiles) {
      const content = fs.readFileSync(pf, 'utf-8');
      const goPkgMatch = content.match(/option\s+go_package\s*=\s*"([^"]+)"/);
      if (goPkgMatch) {
        result.goPackage = goPkgMatch[1];
        break;
      }
    }
  }

  return result;
}

function findModelsDir(workspaceRoot: string, filePath: string): string | null {
  if (filePath.includes('models')) {
    const idx = filePath.replace(/\\/g, '/').indexOf('/models/');
    if (idx >= 0) {
      return filePath.substring(0, idx + '/models'.length);
    }
  }

  const modelsPath = path.join(workspaceRoot, 'models');
  if (fs.existsSync(modelsPath)) {
    return modelsPath;
  }

  return null;
}

function findProtoDir(workspaceRoot: string): string | null {
  const candidates = ['proto', 'protos', 'api/proto', 'api/protos'];
  for (const c of candidates) {
    const p = path.join(workspaceRoot, c);
    if (fs.existsSync(p)) {
      return p;
    }
  }
  return null;
}

function listProtoFiles(dir: string): string[] {
  const results: string[] = [];
  try {
    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        results.push(...listProtoFiles(fullPath));
      } else if (entry.name.endsWith('.proto')) {
        results.push(fullPath);
      }
    }
  } catch {
    // ignore
  }
  return results;
}

export function findGoModInWorkspace(): string[] {
  const results: string[] = [];
  if (!vscode.workspace.workspaceFolders) {
    return results;
  }

  for (const folder of vscode.workspace.workspaceFolders) {
    const goModPath = path.join(folder.uri.fsPath, 'go.mod');
    if (fs.existsSync(goModPath)) {
      results.push(folder.uri.fsPath);
    }

    try {
      const entries = fs.readdirSync(folder.uri.fsPath, { withFileTypes: true });
      for (const entry of entries) {
        if (entry.isDirectory() && !entry.name.startsWith('.') && !entry.name.startsWith('node_modules')) {
          const subGoMod = path.join(folder.uri.fsPath, entry.name, 'go.mod');
          if (fs.existsSync(subGoMod)) {
            results.push(path.join(folder.uri.fsPath, entry.name));
          }
        }
      }
    } catch {
      // ignore
    }
  }

  return results;
}

export function extractProtoPackageFromGoMod(workspaceRoot: string): string {
  const goModPath = path.join(workspaceRoot, 'go.mod');
  if (!fs.existsSync(goModPath)) {
    return '';
  }

  const content = fs.readFileSync(goModPath, 'utf-8');
  const match = content.match(/^module\s+([\S]+)/m);
  if (!match) { return ''; }

  const moduleName = match[1];
  if (moduleName.includes('-share-proto') || moduleName.includes('-proto')) {
    return moduleName;
  }

  const protoMatch = content.match(/(github\.com\/[\w-]+\/[\w-]*proto[\w-]*)/);
  return protoMatch ? protoMatch[1] : '';
}

export function findProtoPackagePath(workspaceRoot: string): string {
  const protoDirs = ['proto', 'protos', 'api/proto'];
  for (const d of protoDirs) {
    const p = path.join(workspaceRoot, d);
    if (fs.existsSync(p)) {
      return p;
    }
  }
  return '';
}
