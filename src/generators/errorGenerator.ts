import { GeneratorConfig } from '../types';
import { camelToSnake } from './index';

export function generateErrorCodes(config: GeneratorConfig): string {
  const { entityName, entityNameLower } = config;
  const snakeName = camelToSnake(entityName);

  return `// ============================================
// ${entityName}相关错误
// ============================================

BizErrCode${entityName}NotFound           = "error.${snakeName}_not_found"
BizErrCode${entityName}CreateFailed       = "error.${snakeName}_create_failed"
BizErrCode${entityName}UpdateFailed       = "error.${snakeName}_update_failed"
BizErrCode${entityName}DeleteFailed       = "error.${snakeName}_delete_failed"
BizErrCode${entityName}UpdateStatusFailed = "error.${snakeName}_update_status_failed"`;
}
