import { GeneratorConfig } from '../types';

export function generateFactoryRegistration(config: GeneratorConfig): string {
  const { entityName } = config;
  const repoName = `${entityName}Repository`;

  return `// === Factory 注册片段 ===
// 1. 在 RepositoryFactory 结构体中添加字段:
${entityName}\t*${repoName}

// 2. 在 NewRepositoryFactory() 中添加初始化:
${entityName}: New${repoName}(),

// 3. 如果需要 getter 方法:
func (f *RepositoryFactory) Get${entityName}Repository() *${repoName} {
\treturn f.${entityName}
}`;
}
