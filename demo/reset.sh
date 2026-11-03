#!/usr/bin/env bash
# 把 demo 复位到基线，便于反复演示
#
# 生成器是幂等的：同一条命令跑第二次会全部跳过 想看一次完整的"从无到有"，
# 就先跑这个脚本
#
# 刻意不去算实体名的 snake 形态
#
# 早先这个脚本自己把 Order 转成 order 来拼文件名，踩了两次：
#  1. sed 的 \L 是 GNU 扩展，macOS 的 BSD sed 不认，Order 被算成 _order，
#     于是 rm 全部打空而脚本看起来还成功了
#  2. 改成 awk 之后 HTTPServer 被算成 h_t_t_p_server，而生成器给的是 http_server
#
# 根因是在 bash 里复刻生成器的命名算法 —— 两份实现迟早漂移
# 现在换成让 git 认：生成出来的新文件是未跟踪的，交给 git clean；
# 被插入条目的是已跟踪文件，交给 git checkout 这样一个名字都不用算
#
# [EN] The entity name's snake form is deliberately not computed here
# This script used to derive the file names itself and got it wrong twice: sed's \L is a GNU
# extension that macOS's BSD sed ignores, turning Order into _order so every rm silently
# missed while the script still looked successful; and the awk replacement turned HTTPServer
# into h_t_t_p_server where the generator produces http_server
# The root cause is reimplementing the generator's naming in bash, where two implementations
# drift apart
# Now git decides instead: the generated files are untracked and go to git clean, while the
# files that received insertions are tracked and go to git checkout, so no name is computed
set -euo pipefail
cd "$(dirname "$0")"

RESTORE=(models/table_names.go repository/factory.go migrations/auto_migrate.go
	service/exports.go locales/en.json locales/zh.json)

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "不在 git 工作树里，无法复位" >&2
	exit 1
fi
for f in "${RESTORE[@]}"; do
	if ! git ls-files --error-unmatch "$f" >/dev/null 2>&1; then
		echo "$f 还没被 git 跟踪，复位需要先提交一次 demo 的基线" >&2
		exit 1
	fi
done

# 只清这三个目录：生成物都落在这里
# 范围刻意收窄 —— git clean 会删未跟踪文件，放开范围会连人随手放的东西一起删
#
# [EN] Only these three directories are cleaned, since that is where generated files land
# The scope is deliberately narrow because git clean removes untracked files and a wider
# scope would take anything a person happened to leave there
git clean -fdq models repository service
git checkout -- "${RESTORE[@]}"
echo "已复位"
