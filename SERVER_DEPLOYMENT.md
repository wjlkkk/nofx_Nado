# NOFX 服务器部署指南

## 快速部署

### 1. 登录到您的服务器

```bash
ssh user@your-server-ip
```

### 2. 克隆代码（如果还没有）

```bash
git clone -b dev https://github.com/wjlkkk/nofx_Nado.git
cd nofx_Nado
```

### 3. 运行部署脚本

```bash
chmod +x deploy-server.sh
./deploy-server.sh
```

就这么简单！脚本会自动：
- 检查环境
- 拉取最新代码
- 构建镜像
- 启动服务

## 访问 NOFX

部署完成后，通过浏览器访问：

```
http://YOUR_SERVER_IP:3000
```

## 配置防火墙

如果无法访问，请确保服务器防火墙允许以下端口：

```bash
# Ubuntu/Debian (ufw)
sudo ufw allow 3000/tcp
sudo ufw allow 8080/tcp

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=3000/tcp
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload
```

## 更新部署

当有新代码时，只需再次运行：

```bash
./deploy-server.sh
```

## 常用管理命令

```bash
# 查看服务状态
docker compose ps

# 查看日志
docker compose logs -f

# 查看后端日志
docker compose logs -f nofx

# 查看前端日志
docker compose logs -f nofx-frontend

# 重启服务
docker compose restart

# 停止服务
docker compose down

# 完全删除服务和数据（谨慎使用）
docker compose down -v
```

## 配置 HTTPS（可选）

### 使用 Cloudflare（推荐）

1. 在 Cloudflare 添加您的域名
2. 创建 DNS 记录指向服务器 IP
3. 设置 SSL/TLS 为 Flexible 模式
4. 启用 `.env` 中的传输加密：

```bash
# 编辑 .env 文件
nano .env

# 添加或修改
TRANSPORT_ENCRYPTION=true

# 重启服务
docker compose restart
```

访问 `https://your-domain.com` 即可

## 数据备份

重要数据存储在 `./data` 目录，建议定期备份：

```bash
# 创建备份
tar -czf nofx-backup-$(date +%Y%m%d).tar.gz data/

# 恢复备份
tar -xzf nofx-backup-YYYYMMDD.tar.gz
```

## 故障排查

### 服务无法启动

```bash
# 查看详细日志
docker compose logs

# 检查端口占用
sudo lsof -i:3000
sudo lsof -i:8080
```

### 端口冲突

修改 `.env` 文件中的端口配置：

```bash
NOFX_BACKEND_PORT=8081
NOFX_FRONTEND_PORT=3001
```

然后重新运行部署脚本。

### 内存不足

如果服务器内存较小（< 2GB），可以限制 Docker 内存使用：

编辑 `/etc/docker/daemon.json`：

```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

重启 Docker：

```bash
sudo systemctl restart docker
```

## 系统要求

- **操作系统**: Linux (Ubuntu 20.04+, Debian 10+, CentOS 7+)
- **内存**: 最低 2GB，推荐 4GB+
- **磁盘**: 最低 10GB 可用空间
- **Docker**: 20.10+
- **Docker Compose**: 2.0+

## 安全建议

1. **修改默认端口**: 避免使用常见端口
2. **启用 HTTPS**: 保护数据传输
3. **定期更新**: 经常运行 `./deploy-server.sh` 更新
4. **备份数据**: 定期备份 `./data` 目录
5. **限制访问**: 使用防火墙限制访问来源

## 获取帮助

- GitHub Issues: https://github.com/NoFxAiOS/nofx/issues
- Telegram 社区: https://t.me/nofx_dev_community
