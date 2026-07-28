# CDK 充值系统 - 快速启动指南

## ✅ 编译成功！

后端和前端都已编译成功。可以直接本地运行，无需 Docker。

### 文件位置

- **后端可执行文件**: `backend/cdk-recharge` (11 MB)
- **前端打包文件**: `frontend/dist/` (包含 HTML + CSS + JS)

---

## 📋 前置要求

### 1. 启动 PostgreSQL 数据库

**选项 A: 本地数据库（推荐）**
```bash
# 确保 PostgreSQL 已启动
sudo systemctl start postgresql

# 创建数据库
createdb cdk_recharge

# 验证
psql -U postgres -d cdk_recharge -c "SELECT NOW();"
```

**选项 B: Docker 容器**
```bash
docker run -d --name cdk_postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=cdk_recharge \
  -p 5432:5432 \
  postgres:15-alpine
```

### 2. 启动 Redis（可选，目前不使用）
```bash
# 本地
redis-server

# 或 Docker
docker run -d --name cdk_redis -p 6379:6379 redis:7-alpine
```

---

## 🚀 运行项目

### 方法 1: 直接运行后端 + 前端开发模式

**终端 1 - 启动后端:**
```bash
cd /home/tuzi/Documents/cdk-recharge-system/backend

# 设置环境变量
export SERVER_HOST=0.0.0.0
export SERVER_PORT=8080
export SERVER_MODE=debug
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=postgres
export DB_NAME=cdk_recharge
export DB_SSLMODE=disable
export REDIS_HOST=localhost
export REDIS_PORT=6379
export JWT_SECRET=dev-secret-key
export JWT_EXPIRATION_HOURS=24

# 运行后端
./cdk-recharge

# 或用 Go 运行（支持代码热更新）
go run ./cmd/server/main.go
```

**终端 2 - 启动前端开发服务器:**
```bash
cd /home/tuzi/Documents/cdk-recharge-system/frontend
npm run dev
```

**终端 3 - 测试 API:**
```bash
# 检查后端健康状态
curl http://localhost:8080/health

# 预期响应:
# {"message":"CDK Recharge System is running","status":"ok"}
```

### 方法 2: 使用启动脚本
```bash
cd /home/tuzi/Documents/cdk-recharge-system

# 使脚本可执行
chmod +x start-dev.sh

# 运行脚本（会同时启动后端和前端）
./start-dev.sh
```

---

## 🌐 访问应用

启动后，访问以下地址：

- **前端界面**: http://localhost:5173
- **后端 API**: http://localhost:8080/api/v1
- **健康检查**: http://localhost:8080/health

---

## 📊 现在的功能

### ✅ 已完成
- [x] 项目结构搭建
- [x] 数据库 Schema 设计
- [x] 后端编译成功
- [x] 前端编译成功
- [x] 路由框架
- [x] 占位符 API 端点（返回模拟数据）
- [x] 中间件框架（CORS, JWT, Admin Auth）
- [x] 现代化首页设计

### ⏳ 待开发（Phase 2+）
- [ ] 完整的认证系统（JWT Token 签发/验证）
- [ ] 用户注册和登录
- [ ] CDK 生成、验证、查询
- [ ] 充值申请提交和审核
- [ ] 用户余额管理
- [ ] 管理后台页面
- [ ] 数据库操作完善

---

## 🧪 测试数据库连接

### 测试 PostgreSQL
```bash
psql -U postgres -d cdk_recharge -c "SELECT * FROM users;"
```

### 查看已创建的表
```bash
psql -U postgres -d cdk_recharge -c "\dt"

# 预期输出:
#           List of relations
# Schema |       Name        | Type  | Owner
#--------+-------------------+-------+-------
# public | cd_keys           | table | postgres
# public | recharge_logs     | table | postgres
# public | recharge_requests | table | postgres
# public | users             | table | postgres
```

---

## 📝 环境配置

所有环境变量已在 `.env.local` 中配置：

```env
SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_MODE=debug
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=cdk_recharge
DB_SSLMODE=disable
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
JWT_SECRET=dev-secret-key-change-in-production
JWT_EXPIRATION_HOURS=24
```

---

## 🔧 开发工作流

### 修改后端代码
```bash
cd backend
go run ./cmd/server/main.go
# 修改后会自动重新编译
```

### 修改前端代码
```bash
cd frontend
npm run dev
# 修改后会自动热更新
```

---

## 📦 生产部署

构建优化的二进制文件：
```bash
cd backend
go build -ldflags="-s -w" -o cdk-recharge ./cmd/server/main.go
# 大小从 11MB 减少到 ~5MB
```

---

## 🐛 故障排除

### 数据库连接错误
```
dial tcp [::1]:5432: connect: connection refused
```
**解决**: 确保 PostgreSQL 已启动
```bash
sudo systemctl start postgresql
# 或检查 Docker 容器
docker ps | grep postgres
```

### 前端编译错误
```bash
# 重新安装依赖
cd frontend
rm -rf node_modules package-lock.json
npm install
npm run dev
```

### 后端编译错误
```bash
# 清理 Go 缓存
cd backend
go clean -cache
go mod tidy
go build ./cmd/server/main.go
```

---

## 📚 后续步骤

现在系统框架已搭建完成，下一步是实现核心业务逻辑：

1. **认证系统** - 注册、登录、JWT Token
2. **CDK 管理** - 生成、验证、使用
3. **充值流程** - 申请、审核、入账
4. **管理后台** - CDK 列表、申请审核

每个功能都可以独立开发和测试！

---

## 🎯 快速命令

```bash
# 进入项目目录
cd /home/tuzi/Documents/cdk-recharge-system

# 启动后端
cd backend && go run ./cmd/server/main.go

# 启动前端
cd frontend && npm run dev

# 构建生产版本
make build

# 查看所有可用命令
make help
```

祝你开发愉快！ 🚀
