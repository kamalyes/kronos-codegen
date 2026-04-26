import { GoModelField, GoModelStruct } from '../types';

export function parseGoModelContent(content: string, selectedText?: string): GoModelStruct | null {
  let structMatch: RegExpMatchArray | null = null;

  if (selectedText && selectedText.trim()) {
    structMatch = selectedText.match(/type\s+(\w+Model)\s+struct\s*\{([\s\S]*?)\}/);
    if (!structMatch) {
      structMatch = selectedText.match(/type\s+(\w+)\s+struct\s*\{([\s\S]*?)\}/);
    }
    if (!structMatch && /[\s\S]*\{[\s\S]*\}/.test(selectedText)) {
      const bodyMatch = selectedText.match(/\{([\s\S]*)\}/);
      if (bodyMatch) {
        const surroundingStruct = content.match(new RegExp(`type\\s+(\\w+(?:Model)?)\\s+struct\\s*\\{[\\s\\S]*?\\{${escapeRegExp(selectedText.substring(0, 30))}`));
        if (surroundingStruct) {
          structMatch = surroundingStruct;
        } else {
          structMatch = ['', guessStructName(content, selectedText), bodyMatch[1]] as RegExpMatchArray;
        }
      }
    }
  }

  if (!structMatch) {
    const modelStructs = findAllModelStructs(content);
    if (modelStructs.length === 1) {
      structMatch = modelStructs[0];
    } else if (modelStructs.length > 1) {
      return null;
    }
  }

  if (!structMatch) {
    structMatch = content.match(/type\s+(\w+Model)\s+struct\s*\{([\s\S]*?)\}/);
  }

  if (!structMatch) {
    return null;
  }

  const structName = structMatch[1];
  const body = structMatch[2];
  const fields = parseStructFields(body);

  if (fields.length === 0) {
    return null;
  }

  const primaryKeyField = findPrimaryKey(fields);
  const tableName = extractTableName(content, structName);
  const tableComment = extractTableComment(content, structName);
  const pbType = extractPBType(content, structName);
  const pbPackage = extractPBPackage(content);
  const enumPackage = extractEnumPackage(content);

  return {
    name: structName,
    tableName,
    tableComment,
    fields,
    primaryKeyField: primaryKeyField?.name || '',
    primaryKeyType: primaryKeyField?.goType || 'string',
    pbType,
    pbPackage,
    enumPackage,
  };
}

export function findAllModelStructs(content: string): RegExpMatchArray[] {
  const results: RegExpMatchArray[] = [];
  const regex = /type\s+(\w+Model)\s+struct\s*\{([\s\S]*?)\}/g;
  let match: RegExpMatchArray | null;
  while ((match = regex.exec(content)) !== null) {
    results.push(match);
  }
  if (results.length === 0) {
    const anyStruct = /type\s+(\w+)\s+struct\s*\{([\s\S]*?)\}/g;
    while ((match = anyStruct.exec(content)) !== null) {
      if (isModelLikeStruct(match[1])) {
        results.push(match);
      }
    }
  }
  return results;
}

export function findStructNames(content: string): string[] {
  const names: string[] = [];
  const seen = new Set<string>();
  const modelRegex = /type\s+(\w+Model)\s+struct\s*\{/g;
  let match: RegExpMatchArray | null;
  while ((match = modelRegex.exec(content)) !== null) {
    if (!seen.has(match[1])) {
      names.push(match[1]);
      seen.add(match[1]);
    }
  }
  if (names.length === 0) {
    const anyStruct = /type\s+(\w+)\s+struct\s*\{/g;
    while ((match = anyStruct.exec(content)) !== null) {
      if (isModelLikeStruct(match[1]) && !seen.has(match[1])) {
        names.push(match[1]);
        seen.add(match[1]);
      }
    }
  }
  return names;
}

function isModelLikeStruct(name: string): boolean {
  const skip = ['PBConvertible', 'RepositoryFactory', 'Pagination', 'Query', 'Filter', 'Options', 'Config', 'Params', 'Request', 'Response', 'Result', 'Handler', 'Middleware', 'Server', 'Client', 'Service', 'Impl'];
  if (skip.some(s => name.includes(s))) {
    return false;
  }
  if (name.endsWith('Model') || name.endsWith('Data') || name.endsWith('Entry') || name.endsWith('Info') || name.endsWith('Item') || name.endsWith('Record')) {
    return true;
  }
  if (name.length > 5 && /^[A-Z]/.test(name) && !name.startsWith('New') && !name.startsWith('Get') && !name.startsWith('Set') && !name.startsWith('Build') && !name.startsWith('Parse')) {
    return true;
  }
  return false;
}

function guessStructName(content: string, selectedText: string): string {
  const beforeSelection = content.substring(0, content.indexOf(selectedText));
  const structHeader = beforeSelection.match(/type\s+(\w+(?:Model)?)\s+struct\s*\{\s*$/);
  if (structHeader) {
    return structHeader[1];
  }
  const fileName = content.match(/package\s+(\w+)/);
  if (fileName) {
    return fileName[1].charAt(0).toUpperCase() + fileName[1].slice(1) + 'Model';
  }
  return 'UnknownModel';
}

function escapeRegExp(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function parseStructFields(body: string): GoModelField[] {
  const fields: GoModelField[] = [];
  const lines = body.split('\n');

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('//') || trimmed.startsWith('/*') || trimmed.startsWith('*')) {
      continue;
    }

    const field = parseFieldLine(trimmed);
    if (field) {
      fields.push(field);
    }
  }

  return fields;
}

function parseFieldLine(line: string): GoModelField | null {
  const gormAndJson = line.match(
    /^(\w+)\s+([\w.*\[\]]+(?:<[\w.*\[\]]+>)?)\s+`[^`]*gorm:"([^"]*)"[^`]*json:"([^"]*)"[^`]*`(?:\s*\/\/\s*(.*))?$/
  );
  if (gormAndJson) {
    return buildFieldFromMatch(gormAndJson);
  }

  const gormOnly = line.match(
    /^(\w+)\s+([\w.*\[\]]+(?:<[\w.*\[\]]+>)?)\s+`[^`]*gorm:"([^"]*)"[^`]*`(?:\s*\/\/\s*(.*))?$/
  );
  if (gormOnly) {
    const name = gormOnly[1];
    const gormTag = gormOnly[2] ? gormOnly[3] : '';
    const jsonName = extractColumnFromGorm(gormOnly[3]) || camelToSnake(name);
    return {
      name,
      goType: gormOnly[2],
      gormTag: gormOnly[3],
      jsonTag: jsonName,
      comment: gormOnly[4] ? gormOnly[4].trim() : '',
      isPrimaryKey: gormOnly[3].includes('primaryKey'),
    };
  }

  const jsonOnly = line.match(
    /^(\w+)\s+([\w.*\[\]]+(?:<[\w.*\[\]]+>)?)\s+`[^`]*json:"([^"]*)"[^`]*`(?:\s*\/\/\s*(.*))?$/
  );
  if (jsonOnly) {
    return {
      name: jsonOnly[1],
      goType: jsonOnly[2],
      gormTag: '',
      jsonTag: jsonOnly[3],
      comment: jsonOnly[4] ? jsonOnly[4].trim() : '',
      isPrimaryKey: jsonOnly[1].endsWith('Id') || jsonOnly[1] === 'ID' || jsonOnly[1] === 'Id',
    };
  }

  const withAnyTag = line.match(
    /^(\w+)\s+([\w.*\[\]]+(?:<[\w.*\[\]]+>)?)\s+`([^`]*)`(?:\s*\/\/\s*(.*))?$/
  );
  if (withAnyTag) {
    const tags = withAnyTag[3];
    const gormMatch = tags.match(/gorm:"([^"]*)"/);
    const jsonMatch = tags.match(/json:"([^"]*)"/);
    const name = withAnyTag[1];
    const gormTag = gormMatch ? gormMatch[1] : '';
    const jsonTag = jsonMatch ? jsonMatch[1] : camelToSnake(name);
    return {
      name,
      goType: withAnyTag[2],
      gormTag,
      jsonTag,
      comment: withAnyTag[4] ? withAnyTag[4].trim() : '',
      isPrimaryKey: gormTag.includes('primaryKey') || name.endsWith('Id') || name === 'ID',
    };
  }

  const noTag = line.match(
    /^(\w+)\s+([\w.*\[\]]+(?:<[\w.*\[\]]+>)?)\s*$/
  );
  if (noTag) {
    const name = noTag[1];
    return {
      name,
      goType: noTag[2],
      gormTag: '',
      jsonTag: camelToSnake(name),
      comment: '',
      isPrimaryKey: name.endsWith('Id') || name === 'ID' || name === 'Id',
    };
  }

  return null;
}

function extractColumnFromGorm(gormTag: string): string {
  const match = gormTag.match(/column:(\w+)/);
  return match ? match[1] : '';
}

function buildFieldFromMatch(match: RegExpMatchArray): GoModelField {
  const name = match[1];
  const goType = match[2];
  const gormTag = match[3];
  const jsonTag = match[4];
  const comment = match[5] ? match[5].trim() : '';

  return {
    name,
    goType,
    gormTag,
    jsonTag,
    comment,
    isPrimaryKey: gormTag.includes('primaryKey'),
  };
}

function findPrimaryKey(fields: GoModelField[]): GoModelField | undefined {
  return fields.find(f => f.isPrimaryKey) || fields.find(f => f.name.endsWith('Id') || f.name.endsWith('ID'));
}

function extractTableName(content: string, structName: string): string {
  const receiverValue = new RegExp(`func\\s+\\(\\w*\\s+${escapeRegExp(structName)}\\)\\s+TableName\\(\\)\\s+string\\s*\\{\\s*return\\s*"([^"]+)"`, 's');
  const receiverPtr = new RegExp(`func\\s+\\(\\w*\\s+\\*${escapeRegExp(structName)}\\)\\s+TableName\\(\\)\\s+string\\s*\\{\\s*return\\s*"([^"]+)"`, 's');

  const match = content.match(receiverValue) || content.match(receiverPtr);
  return match ? match[1] : camelToSnake(structName.replace('Model', '')) + 's';
}

function extractTableComment(content: string, structName: string): string {
  const receiverValue = new RegExp(`func\\s+\\(\\w*\\s+${escapeRegExp(structName)}\\)\\s+TableComment\\(\\)\\s+string\\s*\\{\\s*return\\s*"([^"]+)"`, 's');
  const receiverPtr = new RegExp(`func\\s+\\(\\w*\\s+\\*${escapeRegExp(structName)}\\)\\s+TableComment\\(\\)\\s+string\\s*\\{\\s*return\\s*"([^"]+)"`, 's');

  const match = content.match(receiverValue) || content.match(receiverPtr);
  if (match) {
    return match[1];
  }

  const commentAbove = content.match(new RegExp(`//\\s*(\\S[^\\n]*)\\ntype\\s+${escapeRegExp(structName)}\\s+struct`));
  if (commentAbove) {
    let comment = commentAbove[1].trim();
    comment = comment.replace(new RegExp(escapeRegExp(structName), 'g'), '').trim();
    comment = comment.replace(/^[-–—\s]+/, '').trim();
    if (comment) {
      return comment;
    }
  }

  return '';
}

function extractPBType(content: string, structName: string): string {
  const regex = new RegExp(`pbmo\\.Register\\[${escapeRegExp(structName)},\\s*pb\\.(\\w+)\\]`, 's');
  const match = content.match(regex);
  return match ? match[1] : structName.replace('Model', 'Info');
}

function extractPBPackage(content: string): string {
  const match = content.match(/pb\s+"([^"]+)"/);
  return match ? match[1] : '';
}

function extractEnumPackage(content: string): string {
  const match = content.match(/enumspb\s+"([^"]+)"/);
  return match ? match[1] : '';
}

function camelToSnake(str: string): string {
  return str.replace(/([A-Z])/g, '_$1').toLowerCase().replace(/^_/, '');
}

export function modelToFieldConfigs(model: GoModelStruct): import('../types').FieldConfig[] {
  return model.fields.map(f => {
    const isEnum = f.goType.includes('enumspb.') || f.goType.includes('enums.');
    const enumType = isEnum ? f.goType.split('.').pop() || '' : '';
    const isCreatedAt = f.name === 'CreatedAt' || f.gormTag.includes('autoCreateTime');
    const isUpdatedAt = f.name === 'UpdatedAt' || f.gormTag.includes('autoUpdateTime');
    const isI18N = f.goType.includes('LocalizedTextList') || f.goType.includes('i18n');

    return {
      protoName: camelToSnake(f.name),
      goName: f.name,
      goType: f.goType,
      gormTag: f.gormTag,
      jsonTag: f.jsonTag,
      comment: f.comment,
      isPrimaryKey: f.isPrimaryKey,
      isEnum,
      enumType,
      protoType: goTypeToProtoType(f.goType),
      isUpdatedAt,
      isCreatedAt,
      isI18N,
      isOptionalWrapper: false,
    };
  });
}

function goTypeToProtoType(goType: string): string {
  if (goType.includes('enumspb.') || goType.includes('enums.')) {
    return goType.split('.').pop() || 'int32';
  }
  if (goType.includes('LocalizedTextList') || goType.includes('i18n')) {
    return 'repeated common.LocalizedText';
  }
  if (goType.startsWith('[]')) {
    const inner = goType.slice(2);
    return `repeated ${goTypeToProtoType(inner)}`;
  }
  const map: Record<string, string> = {
    'string': 'string',
    'int32': 'int32',
    'int64': 'int64',
    'uint32': 'uint32',
    'uint64': 'uint64',
    'bool': 'bool',
    'float32': 'float',
    'float64': 'double',
    'time.Time': 'google.protobuf.Timestamp',
    '[]byte': 'bytes',
  };
  return map[goType] || 'string';
}
