#!/bin/bash

echo "================================================"
echo "CDK 充值系统 - 本地开发启动"
echo "================================================"
echo ""

# 检查环境变量文件
if [ ! -f .env.local ]; then
    echo "✗ 未找到 .env.local 文件"
    exit 1
fi

echo "✓ 环境文件已找到"
echo ""

# 检查数据库连接
echo "检查数据库连接..."
psql -U postgres -h localhost -d cdk_recharge -c "SELECT NOW();" >/dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "✗ PostgreSQL 连接失败"
    echo "  请确保 PostgreSQL 已启动，或运行:"
    echo "  createdb cdk_recharge"
    exit 1
fi
echo "✓ PostgreSQL 连接成功"

# 检查 Redis
redis-cli ping >/dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "✗ Redis 连接失败 (可选，不影响基本功能)"
else
    echo "✓ Redis 连接成功"
fi

echo ""
echo "启动服务..."
echo ""

# 启动后端
cd backend
export $(cat ../.env.local | xargs)
echo "启动后端服务 (端口 8080)..."
go run ./cmd/server/main.go &
BACKEND_PID=$!
echo "  后端 PID: $BACKEND_PID"

sleep 2

# 启动前端
cd ../frontend
echo "启动前端服务 (端口 5173)..."
npm run dev &
FRONTEND_PID=$!
echo "  前端 PID: $FRONTEND_PID"

echo ""
echo "================================================"
echo "✓ 服务已启动！"
echo "================================================"
echo ""
echo "前端:    http://localhost:5173"
echo "后端:    http://localhost:8080"
echo "API:     http://localhost:8080/api/v1"
echo "健康检查: curl http://localhost:8080/health"
echo ""
echo "按 Ctrl+C 停止所有服务..."
echo ""

# 等待中断信号
trap "kill $BACKEND_PID $FRONTEND_PID; exit" SIGINT SIGTERM

# 保持脚本运行
wait
