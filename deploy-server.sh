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

echo -e "${YELLOW}[1/5] 检查环境...${NC}"

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

echo -e "${YELLOW}[2/5] 拉取最新代码...${NC}"
git pull nado dev
echo -e "${GREEN}✓ 代码已更新${NC}"

echo -e "${YELLOW}[3/5] 停止现有服务...${NC}"
docker compose down 2>/dev/null || true
echo -e "${GREEN}✓ 服务已停止${NC}"

echo -e "${YELLOW}[4/5] 构建镜像...${NC}"
# 使用 --no-cache 确保构建最新代码
docker compose build --no-cache
echo -e "${GREEN}✓ 镜像构建完成${NC}"

echo -e "${YELLOW}[5/5] 启动服务...${NC}"
docker compose up -d
echo -e "${GREEN}✓ 服务已启动${NC}"

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}部署完成！${NC}"
echo ""
echo -e "访问地址:"
echo -e "  - Web 界面: ${BLUE}http://YOUR_SERVER_IP:3000${NC}"
echo -e "  - API: ${BLUE}http://YOUR_SERVER_IP:8080${NC}"
echo ""
echo -e "常用命令:"
echo -e "  - 查看日志: ${YELLOW}docker compose logs -f${NC}"
echo -e "  - 重启服务: ${YELLOW}docker compose restart${NC}"
echo -e "  - 停止服务: ${YELLOW}docker compose down${NC}"
echo -e "  - 查看状态: ${YELLOW}docker compose ps${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════${NC}"
