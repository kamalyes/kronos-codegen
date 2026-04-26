import { ProtoField, ProtoMessage, ProtoService, ProtoMethod, ParsedProto } from '../types';

export function parseProtoContent(content: string): ParsedProto {
  const result: ParsedProto = {
    package: '',
    goPackage: '',
    messages: [],
    services: [],
  };

  result.package = extractPackage(content);
  result.goPackage = extractGoPackage(content);
  result.messages = extractMessages(content);
  result.services = extractServices(content);

  return result;
}

function extractPackage(content: string): string {
  const match = content.match(/^package\s+([\w.]+)\s*;/m);
  return match ? match[1] : '';
}

function extractGoPackage(content: string): string {
  const match = content.match(/^option\s+go_package\s*=\s*"([^"]+)"/m);
  return match ? match[1] : '';
}

function extractMessages(content: string): ProtoMessage[] {
  const messages: ProtoMessage[] = [];
  const messageRegex = /(?:\/\/\s*(.*?)\n\s*)?message\s+(\w+)\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}/gs;

  let match;
  while ((match = messageRegex.exec(content)) !== null) {
    const comment = match[1] ? match[1].trim() : '';
    const name = match[2];
    const body = match[3];
    const fields = extractFields(body);
    messages.push({ name, fields, comment });
  }

  return messages;
}

function extractFields(body: string): ProtoField[] {
  const fields: ProtoField[] = [];
  const lines = body.split('\n');

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('//') || trimmed.startsWith('option') || trimmed.startsWith('reserved') || trimmed.startsWith('oneof')) {
      continue;
    }

    const fieldMatch = trimmed.match(
      /^(repeated\s+)?(optional\s+)?([\w.]+)\s+(\w+)\s*=\s*(\d+)\s*;\s*(?:\/\/\s*(.*))?$/
    );

    if (fieldMatch) {
      fields.push({
        repeated: !!fieldMatch[1],
        optional: !!fieldMatch[2],
        type: fieldMatch[3],
        name: fieldMatch[4],
        number: parseInt(fieldMatch[5], 10),
        comment: fieldMatch[6] ? fieldMatch[6].trim() : '',
      });
    }
  }

  return fields;
}

function extractServices(content: string): ProtoService[] {
  const services: ProtoService[] = [];
  const serviceRegex = /service\s+(\w+)\s*\{([^}]*)\}/gs;

  let match;
  while ((match = serviceRegex.exec(content)) !== null) {
    const name = match[1];
    const body = match[2];
    const methods = extractMethods(body);
    services.push({ name, methods });
  }

  return services;
}

function extractMethods(body: string): ProtoMethod[] {
  const methods: ProtoMethod[] = [];
  const methodRegex = /rpc\s+(\w+)\s*\(\s*([\w.]+)\s*\)\s*returns\s*\(\s*([\w.]+)\s*\)/g;

  let match;
  while ((match = methodRegex.exec(body)) !== null) {
    methods.push({
      name: match[1],
      inputType: match[2],
      outputType: match[3],
    });
  }

  return methods;
}

export function findInfoMessage(messages: ProtoMessage[], entityName: string): ProtoMessage | undefined {
  const normalizedEntity = entityName.toLowerCase();

  return messages.find(m => {
    const msgName = m.name.toLowerCase();
    return msgName === `${normalizedEntity}info` ||
           msgName === `${normalizedEntity}` ||
           msgName === normalizedEntity.replace(/_/g, '');
  });
}

export function findCrudMessages(messages: ProtoMessage[], entityName: string): {
  info?: ProtoMessage;
  createReq?: ProtoMessage;
  updateReq?: ProtoMessage;
  listReq?: ProtoMessage;
} {
  const normalizedEntity = entityName.charAt(0).toUpperCase() + entityName.slice(1);

  return {
    info: messages.find(m => m.name === `${normalizedEntity}Info`),
    createReq: messages.find(m => m.name === `${normalizedEntity}CreateRequest`),
    updateReq: messages.find(m => m.name === `${normalizedEntity}UpdateRequest`),
    listReq: messages.find(m => m.name === `${normalizedEntity}ListRequest`),
  };
}

export function protoTypeToGoType(protoType: string, repeated: boolean): string {
  const typeMap: Record<string, string> = {
    'string': 'string',
    'int32': 'int32',
    'int64': 'int64',
    'uint32': 'uint32',
    'uint64': 'uint64',
    'bool': 'bool',
    'float': 'float32',
    'double': 'float64',
    'bytes': '[]byte',
    'google.protobuf.Timestamp': 'time.Time',
    'google.protobuf.StringValue': 'string',
    'google.protobuf.Int32Value': 'int32',
    'google.protobuf.Int64Value': 'int64',
    'google.protobuf.BoolValue': 'bool',
    'google.protobuf.Struct': 'string',
  };

  let goType = typeMap[protoType] || protoType;

  if (repeated && !goType.startsWith('[]')) {
    goType = `[]${goType}`;
  }

  return goType;
}

export function inferGormTag(fieldName: string, protoType: string, fieldNumber: number, isRepeated: boolean): string {
  const snakeCase = camelToSnake(fieldName);
  const parts: string[] = [`column:${snakeCase}`];

  if (fieldName.endsWith('_id') || fieldNumber === 1) {
    parts.push('type:varchar(36)', 'primaryKey');
  } else if (protoType === 'string' && !isRepeated) {
    if (fieldName === 'description' || fieldName === 'config') {
      parts.push('type:text');
    } else if (fieldName.includes('email') || fieldName.includes('account')) {
      parts.push(`type:varchar(128)`);
    } else if (snakeCase.length > 64) {
      parts.push('type:varchar(256)');
    } else {
      parts.push('type:varchar(64)');
    }
    if (fieldNumber === 1 || fieldName.endsWith('_id')) {
      // already handled
    } else if (fieldName.includes('name') || fieldName === 'code') {
      parts.push('not null');
    } else {
      parts.push("default:''");
    }
  } else if (protoType === 'int32' || protoType === 'int64' || protoType === 'uint32' || protoType === 'uint64') {
    if (protoType.includes('64')) {
      parts.push('type:bigint');
    } else {
      parts.push('type:int');
    }
    if (fieldName.includes('sort_order')) {
      parts.push('default:0');
    }
  } else if (protoType === 'bool') {
    parts.push('type:tinyint', 'default:0');
  } else if (protoType === 'google.protobuf.Timestamp') {
    parts.push('autoCreateTime');
  } else if (isRepeated) {
    parts.push('type:json');
  }

  if (fieldName.includes('tenant_id') || fieldName.includes('_id')) {
    if (fieldNumber !== 1) {
      parts.push('index');
    }
  }

  parts.push(`comment:${fieldName}`);

  return parts.join(';');
}

function camelToSnake(str: string): string {
  return str.replace(/([A-Z])/g, '_$1').toLowerCase().replace(/^_/, '');
}
