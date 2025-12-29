#!/bin/bash

# NOFX 服务器部署脚本
# 用途：在远端服务器上部署 NOFX

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}"
echo "╔════════════════════════════════════════════════════════════╗"
echo "║         NOFX - 远端服务器部署脚本                          ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# 检查 Docker 是否安装
if ! command -v docker &> /dev/null; then
    echo -e "${RED}错误: Docker 未安装${NC}"
    echo "请先安装 Docker: https://docs.docker.com/get-docker/"
    exit 1
fi

# 检查 Docker Compose 是否可用
if ! command -v docker compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo -e "${RED}错误: Docker Compose 未安装${NC}"
    exit 1
fi

# 获取当前目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo -e "${YELLOW}[1/6] 检查环境...${NC}"

# 检查是否存在 .env 文件
if [ ! -f .env ]; then
    echo -e "${YELLOW}未找到 .env 文件，从 .env.example 复制...${NC}"
    if [ -f .env.example ]; then
        cp .env.example .env
        echo -e "${GREEN}✓ 已创建 .env 文件${NC}"
        echo -e "${YELLOW}请编辑 .env 文件，配置您的密钥${NC}"
    else
        echo -e "${RED}错误: .env.example 文件不存在${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ .env 文件已存在${NC}"
fi

echo -e "${YELLOW}[2/6] 检查端口占用...${NC}"

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
BACKEND_PORT=$(grep "^NOFX_BACKEND_PORT=" .env | cut -d'=' -f2)
FRONTEND_PORT=$(grep "^NOFX_FRONTEND_PORT=" .env | cut -d'=' -f2)

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
        # 更新 .env 文件
        sed -i "s/^NOFX_BACKEND_PORT=.*/NOFX_BACKEND_PORT=$BACKEND_PORT/" .env
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
        sed -i "s/^NOFX_FRONTEND_PORT=.*/NOFX_FRONTEND_PORT=$FRONTEND_PORT/" .env
    else
        echo -e "${RED}错误: 无法找到可用端口${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ 前端端口 $FRONTEND_PORT 可用${NC}"
fi

echo -e "${YELLOW}[3/6] 拉取最新代码...${NC}"
git pull nado dev
echo -e "${GREEN}✓ 代码已更新${NC}"

echo -e "${YELLOW}[4/6] 停止现有服务...${NC}"
docker compose down 2>/dev/null || true
echo -e "${GREEN}✓ 服务已停止${NC}"

echo -e "${YELLOW}[5/6] 构建镜像...${NC}"
# 使用 --no-cache 确保构建最新代码
docker compose build --no-cache
echo -e "${GREEN}✓ 镜像构建完成${NC}"

echo -e "${YELLOW}[6/6] 启动服务...${NC}"
docker compose up -d
echo -e "${GREEN}✓ 服务已启动${NC}"

# 获取服务器 IP
SERVER_IP=$(curl -s ifconfig.me || curl -s icanhazip.com || hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}部署完成！${NC}"
echo ""
echo -e "访问地址:"
echo -e "  - Web 界面: ${BLUE}http://${SERVER_IP}:${FRONTEND_PORT}${NC}"
echo -e "  - API: ${BLUE}http://${SERVER_IP}:${BACKEND_PORT}${NC}"
echo ""
if [ "$FRONTEND_PORT" != "3000" ] || [ "$BACKEND_PORT" != "8080" ]; then
    echo -e "${YELLOW}注意: 使用了非默认端口，请使用上述端口访问${NC}"
    echo ""
fi
echo -e "常用命令:"
echo -e "  - 查看日志: ${YELLOW}docker compose logs -f${NC}"
echo -e "  - 重启服务: ${YELLOW}docker compose restart${NC}"
echo -e "  - 停止服务: ${YELLOW}docker compose down${NC}"
echo -e "  - 查看状态: ${YELLOW}docker compose ps${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
