#!/bin/bash

# NOFX 服务器部署脚本 (改进版)
# 用途：在远端服务器上部署 NOFX

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════╗"
echo "║         NOFX - 远端服务器部署脚本 v2.0                     ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# 获取当前目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo -e "${YELLOW}[1/7] 检查环境...${NC}"

# 检查 Docker 是否安装
if ! command -v docker &> /dev/null; then
    echo -e "${RED}错误: Docker 未安装${NC}"
    echo "请先安装 Docker: https://docs.docker.com/get-docker/"
    exit 1
fi
echo -e "${GREEN}✓ Docker 已安装${NC}"

# 检查 Docker 是否运行
if ! docker info &> /dev/null; then
    echo -e "${RED}错误: Docker daemon 未运行${NC}"
    echo "请启动 Docker: sudo systemctl start docker"
    exit 1
fi
echo -e "${GREEN}✓ Docker 运行中${NC}"

# 检查 Docker Compose
if docker compose version &> /dev/null; then
    COMPOSE_CMD="docker compose"
elif command -v docker-compose &> /dev/null; then
    COMPOSE_CMD="docker-compose"
else
    echo -e "${RED}错误: Docker Compose 未安装${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker Compose 可用: ${COMPOSE_CMD}${NC}"

echo -e "${YELLOW}[2/7] 检查 Git 仓库...${NC}"

# 检查 git remote
if git remote | grep -q "nado"; then
    REMOTE_NAME="nado"
elif git remote | grep -q "origin"; then
    REMOTE_NAME="origin"
    echo -e "${YELLOW}⚠️  未找到 'nado' remote，使用 'origin'${NC}"
else
    echo -e "${RED}错误: 未找到 git remote${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Git remote: ${REMOTE_NAME}${NC}"

echo -e "${YELLOW}[3/7] 检查配置文件...${NC}"

# 检查是否存在 .env 文件
if [ ! -f .env ]; then
    echo -e "${YELLOW}未找到 .env 文件，从 .env.example 复制...${NC}"
    if [ -f .env.example ]; then
        cp .env.example .env

        # 生成加密密钥
        echo -e "${YELLOW}生成加密密钥...${NC}"
        JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || echo "default-jwt-secret-please-change-in-production")
        DATA_ENCRYPTION_KEY=$(openssl rand -base64 32 2>/dev/null || echo "default-encryption-key-please-change-in-production")
        RSA_KEY=$(openssl genrsa 2048 2>/dev/null | tr '\n' '\\' | sed 's/\\/\\n/g' | sed 's/\\n$//' || echo "")

        # 更新 .env 文件
        if [ -n "$JWT_SECRET" ]; then
            sed -i "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" .env 2>/dev/null || \
            sed -i.bak "s/^JWT_SECRET=.*/JWT_SECRET=${JWT_SECRET}/" .env && rm -f .env.bak
        fi

        if [ -n "$DATA_ENCRYPTION_KEY" ]; then
            sed -i "s/^DATA_ENCRYPTION_KEY=.*/DATA_ENCRYPTION_KEY=${DATA_ENCRYPTION_KEY}/" .env 2>/dev/null || \
            sed -i.bak "s/^DATA_ENCRYPTION_KEY=.*/DATA_ENCRYPTION_KEY=${DATA_ENCRYPTION_KEY}/" .env && rm -f .env.bak
        fi

        if [ -n "$RSA_KEY" ]; then
            sed -i "s|^RSA_PRIVATE_KEY=.*|RSA_PRIVATE_KEY=${RSA_KEY}|" .env 2>/dev/null || \
            sed -i.bak "s|^RSA_PRIVATE_KEY=.*|RSA_PRIVATE_KEY=${RSA_KEY}|" .env && rm -f .env.bak
        fi

        echo -e "${GREEN}✓ 已创建并配置 .env 文件${NC}"
    else
        echo -e "${RED}错误: .env.example 文件不存在${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ .env 文件已存在${NC}"
fi

echo -e "${YELLOW}[4/7] 检查端口占用...${NC}"

# 函数：检查端口是否被占用
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1 || \
       netstat -an 2>/dev/null | grep ":$port " | grep LISTEN >/dev/null || \
       ss -ln 2>/dev/null | grep ":$port " >/dev/null; then
        return 0  # 端口被占用
    else
        return 1  # 端口可用
    fi
}

# 函数：查找可用端口
find_available_port() {
    local start_port=$1
    local port=$start_port
    local max_attempts=100

    for ((i=0; i<max_attempts; i++)); do
        if ! check_port $port; then
            echo $port
            return 0
        fi
        ((port++))
    done

    return 1
}

# 读取当前配置的端口
BACKEND_PORT=$(grep "^NOFX_BACKEND_PORT=" .env 2>/dev/null | cut -d'=' -f2)
FRONTEND_PORT=$(grep "^NOFX_FRONTEND_PORT=" .env 2>/dev/null | cut -d'=' -f2)

# 设置默认端口（如果未配置）
BACKEND_PORT=${BACKEND_PORT:-18080}
FRONTEND_PORT=${FRONTEND_PORT:-13000}

# 检查并调整端口
if check_port $BACKEND_PORT; then
    echo -e "${YELLOW}⚠️  后端端口 $BACKEND_PORT 被占用，查找可用端口...${NC}"
    NEW_PORT=$(find_available_port 18080)
    if [ $? -eq 0 ]; then
        BACKEND_PORT=$NEW_PORT
        echo -e "${GREEN}✓ 使用后端端口: $BACKEND_PORT${NC}"
        # 更新 .env 文件（兼容 macOS 和 Linux）
        sed -i "s/^NOFX_BACKEND_PORT=.*/NOFX_BACKEND_PORT=$BACKEND_PORT/" .env 2>/dev/null || \
        sed -i.bak "s/^NOFX_BACKEND_PORT=.*/NOFX_BACKEND_PORT=$BACKEND_PORT/" .env && rm -f .env.bak
    else
        echo -e "${RED}错误: 无法找到可用端口${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ 后端端口 $BACKEND_PORT 可用${NC}"
fi

if check_port $FRONTEND_PORT; then
    echo -e "${YELLOW}⚠️  前端端口 $FRONTEND_PORT 被占用，查找可用端口...${NC}"
    NEW_PORT=$(find_available_port 13000)
    if [ $? -eq 0 ]; then
        FRONTEND_PORT=$NEW_PORT
        echo -e "${GREEN}✓ 使用前端端口: $FRONTEND_PORT${NC}"
        # 更新 .env 文件
        sed -i "s/^NOFX_FRONTEND_PORT=.*/NOFX_FRONTEND_PORT=$FRONTEND_PORT/" .env 2>/dev/null || \
        sed -i.bak "s/^NOFX_FRONTEND_PORT=.*/NOFX_FRONTEND_PORT=$FRONTEND_PORT/" .env && rm -f .env.bak
    else
        echo -e "${RED}错误: 无法找到可用端口${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ 前端端口 $FRONTEND_PORT 可用${NC}"
fi

echo -e "${YELLOW}[5/7] 拉取最新代码...${NC}"
if git pull $REMOTE_NAME dev; then
    echo -e "${GREEN}✓ 代码已更新${NC}"
else
    echo -e "${YELLOW}⚠️  Git pull 失败，继续使用本地代码${NC}"
fi

echo -e "${YELLOW}[6/7] 停止现有服务...${NC}"
$COMPOSE_CMD down 2>/dev/null || true
echo -e "${GREEN}✓ 服务已停止${NC}"

echo -e "${YELLOW}[7/7] 构建并启动服务...${NC}"
echo -e "${CYAN}这可能需要 5-15 分钟，请耐心等待...${NC}"

# 构建镜像（带进度提示）
if $COMPOSE_CMD build --no-cache 2>&1 | tee /tmp/nofx-build.log; then
    echo -e "${GREEN}✓ 镜像构建完成${NC}"
else
    echo -e "${RED}✗ 镜像构建失败${NC}"
    echo -e "${YELLOW}查看构建日志: cat /tmp/nofx-build.log${NC}"
    exit 1
fi

# 启动服务
if $COMPOSE_CMD up -d; then
    echo -e "${GREEN}✓ 服务已启动${NC}"
else
    echo -e "${RED}✗ 服务启动失败${NC}"
    echo -e "${YELLOW}查看日志: $COMPOSE_CMD logs${NC}"
    exit 1
fi

echo -e "${YELLOW}等待服务就绪...${NC}"
sleep 5

# 检查容器状态
echo ""
echo -e "${CYAN}容器状态:${NC}"
$COMPOSE_CMD ps

# 获取服务器 IP
SERVER_IP=$(curl -s --max-time 3 ifconfig.me 2>/dev/null || curl -s --max-time 3 icanhazip.com 2>/dev/null || hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}✓ 部署完成！${NC}"
echo ""
echo -e "访问地址:"
echo -e "  - Web 界面: ${BLUE}http://${SERVER_IP}:${FRONTEND_PORT}${NC}"
echo -e "  - API: ${BLUE}http://${SERVER_IP}:${BACKEND_PORT}${NC}"
echo ""
if [ "$FRONTEND_PORT" != "3000" ] || [ "$BACKEND_PORT" != "8080" ]; then
    echo -e "${YELLOW}注意: 使用了非默认端口${NC}"
    echo ""
fi
echo -e "常用命令:"
echo -e "  - 查看日志: ${YELLOW}${COMPOSE_CMD} logs -f${NC}"
echo -e "  - 查看后端日志: ${YELLOW}${COMPOSE_CMD} logs -f nofx${NC}"
echo -e "  - 查看前端日志: ${YELLOW}${COMPOSE_CMD} logs -f nofx-frontend${NC}"
echo -e "  - 重启服务: ${YELLOW}${COMPOSE_CMD} restart${NC}"
echo -e "  - 停止服务: ${YELLOW}${COMPOSE_CMD} down${NC}"
echo -e "  - 查看状态: ${YELLOW}${COMPOSE_CMD} ps${NC}"
echo ""
echo -e "故障排查:"
echo -e "  - 构建日志: ${YELLOW}cat /tmp/nofx-build.log${NC}"
echo -e "  - 详细日志: ${YELLOW}${COMPOSE_CMD} logs --tail=100${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
