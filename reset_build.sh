#!/bin/bash

# 重置卡住的构建状态
echo "正在重置卡住的构建状态..."

sqlite3 data/fishing.db "UPDATE projects SET build_status='pending', task_id='', docker_image='' WHERE build_status='building';"

echo "✅ 已重置所有卡住的构建状态"
echo ""
echo "现在可以重新尝试构建项目了。"
