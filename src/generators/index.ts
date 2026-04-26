import { GeneratorConfig, GeneratedCode, FieldConfig } from '../types';
import { generateModel } from './modelGenerator';
import { generateRepository } from './repositoryGenerator';
import { generateServiceStyleA } from './serviceGeneratorA';
import { generateServiceStyleB } from './serviceGeneratorB';
import { generateErrorCodes } from './errorGenerator';
import { generateFactoryRegistration } from './factoryGenerator';

export function generateAll(config: GeneratorConfig): GeneratedCode[] {
  const results: GeneratedCode[] = [];
  const modelsDir = config.modelsPackageName || 'models';
  const repoDir = config.repositoryPackageName || 'repository';
  const serviceDir = config.servicePackageName || 'service';
  const errorsDir = config.errorsPackageName || 'errors';

  results.push({
    label: 'Model',
    language: 'go',
    code: generateModel(config),
    fileName: `${modelsDir}/${config.entityNameLower}_model.go`,
  });

  results.push({
    label: 'Repository',
    language: 'go',
    code: generateRepository(config),
    fileName: `${repoDir}/${config.entityNameLower}_repository.go`,
  });

  if (config.serviceStyle === 'single') {
    results.push({
      label: 'Service (单结构体风格)',
      language: 'go',
      code: generateServiceStyleA(config),
      fileName: `${serviceDir}/${config.entityNameLower}_service.go`,
    });
  } else {
    results.push({
      label: 'Service (独立结构体风格)',
      language: 'go',
      code: generateServiceStyleB(config),
      fileName: `${serviceDir}/${config.entityNameLower}_service.go`,
    });
  }

  results.push({
    label: 'Error Codes',
    language: 'go',
    code: generateErrorCodes(config),
    fileName: `${errorsDir}/codes.go`,
  });

  results.push({
    label: 'Factory 注册',
    language: 'go',
    code: generateFactoryRegistration(config),
    fileName: `${repoDir}/factory.go`,
  });

  return results;
}

export function generateFileHeader(config: GeneratorConfig, layerDesc: string): string {
  if (!config.enableFileHeader) {
    return '';
  }

  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  const dateStr = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;
  const entityDesc = config.tableComment || config.entityName;
  const fullDesc = `${entityDesc}${layerDesc}`;
  const author = config.author || 'kronos-codegen';
  const filePath = config.filePath || '';

  const filePathLine = filePath ? `\n * @FilePath: ${filePath}` : '';

  return `/*
 * @Author: ${author}
 * @Date: ${dateStr}
 * @LastEditors: ${author}
 * @LastEditTime: ${dateStr}${filePathLine}
 * @Description: ${fullDesc} - 由 Kronos Code Generator 自动生成
 *
 * Copyright (c) ${now.getFullYear()} by ${config.publisher || 'kronos-team'}, All Rights Reserved.
 */`;
}

export function pascalCase(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

export function camelToSnake(str: string): string {
  return str.replace(/([A-Z])/g, '_$1').toLowerCase().replace(/^_/, '');
}

export function snakeToCamel(str: string): string {
  return str.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

export function isTimestampField(field: FieldConfig): boolean {
  return field.goType === 'time.Time' || field.protoType === 'google.protobuf.Timestamp';
}

export function isSkipField(field: FieldConfig): boolean {
  return field.isCreatedAt || field.isUpdatedAt;
}

export function isBusinessField(field: FieldConfig): boolean {
  return !field.isPrimaryKey && !field.isCreatedAt && !field.isUpdatedAt;
}

export function getCreateFields(fields: FieldConfig[]): FieldConfig[] {
  return fields.filter(f => !f.isCreatedAt && !f.isUpdatedAt);
}

export function getUpdateFields(fields: FieldConfig[]): FieldConfig[] {
  return fields.filter(f => !f.isPrimaryKey && !f.isCreatedAt && !f.isUpdatedAt);
}

export function getListFilterFields(fields: FieldConfig[]): FieldConfig[] {
  return fields.filter(f =>
    !f.isPrimaryKey &&
    !f.isCreatedAt &&
    !f.isUpdatedAt &&
    (f.goType === 'string' || f.isEnum || f.goType === 'int32' || f.goType === 'bool')
  );
}
