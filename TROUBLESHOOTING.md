# NOFX 部署故障排查指南

## 常见问题及解决方案

### 1. 端口被占用 (Error: address already in use)

**症状**:
```
Error response from daemon: failed to bind host port for 0.0.0.0:8080: address already in use
```

**解决方案**:

#### 方案 A: 使用自动端口检测（推荐）
部署脚本会自动检测并切换端口，无需手动操作。

#### 方案 B: 手动修改端口
```bash
# 编辑 .env 文件
nano .env

# 修改端口为未被占用的端口
NOFX_BACKEND_PORT=18080
NOFX_FRONTEND_PORT=13000

# 重新部署
./deploy-server.sh
```

#### 方案 C: 查找并停止占用端口的进程
```bash
# 查找占用端口的进程
sudo lsof -i:8080
sudo lsof -i:3000

# 停止进程
sudo kill -9 <PID>

# 或者停止占用端口的 Docker 容器
docker ps | grep <port>
docker stop <container_id>
```

---

### 2. Docker 构建失败

**症状**:
```
failed to solve: DeadlineExceeded: failed to fetch anonymous token
```

**原因**: 网络问题，无法从 Docker Hub 拉取镜像

**解决方案**:

#### 方案 A: 配置 Docker 镜像加速器（推荐）

**创建或编辑 Docker 配置文件**:
```bash
sudo nano /etc/docker/daemon.json
```

**添加以下内容**:
```json
{
  "registry-mirrors": [
    "https://docker.m.daocloud.io",
    "https://dockerproxy.com",
    "https://docker.mirrors.ustc.edu.cn",
    "https://docker.nju.edu.cn"
  ],
  "dns": ["8.8.8.8", "114.114.114.114"]
}
```

**重启 Docker**:
```bash
sudo systemctl daemon-reload
sudo systemctl restart docker
```

**验证配置**:
```bash
docker info | grep -A 10 "Registry Mirrors"
```

#### 方案 B: 使用代理
```bash
# 配置 Docker 代理
sudo mkdir -p /etc/systemd/system/docker.service.d
sudo nano /etc/systemd/system/docker.service.d/http-proxy.conf
```

添加：
```ini
[Service]
Environment="HTTP_PROXY=http://your-proxy:port"
Environment="HTTPS_PROXY=http://your-proxy:port"
Environment="NO_PROXY=localhost,127.0.0.1"
```

重启 Docker：
```bash
sudo systemctl daemon-reload
sudo systemctl restart docker
```

#### 方案 C: 手动拉取基础镜像
```bash
# 手动拉取所需镜像
docker pull golang:1.25-alpine
docker pull alpine:latest
docker pull node:18-alpine

# 然后重新运行部署脚本
./deploy-server.sh
```

---

### 3. Git pull 失败

**症状**:
```
fatal: 'nado' does not appear to be a git repository
```

**解决方案**:

```bash
# 查看当前 remote
git remote -v

# 如果没有 nado，添加它
git remote add nado https://github.com/wjlkkk/nofx_Nado.git

# 或者修改现有 remote
git remote rename origin old-origin
git remote add origin https://github.com/wjlkkk/nofx_Nado.git

# 重新尝试
git pull nado dev
```

---

### 4. 服务启动失败

**症状**:
容器启动后立即退出

**诊断步骤**:

```bash
# 1. 查看容器状态
docker compose ps

# 2. 查看详细日志
docker compose logs --tail=100

# 3. 查看特定服务的日志
docker compose logs nofx
docker compose logs nofx-frontend

# 4. 查看容器退出代码
docker inspect <container_id> | grep -A 10 "State"
```

**常见原因**:

#### A. 配置文件错误
```bash
# 检查 .env 文件
cat .env

# 确保所有必需的密钥都存在
grep -E "JWT_SECRET|DATA_ENCRYPTION_KEY|RSA_PRIVATE_KEY" .env
```

#### B. 数据库权限问题
```bash
# 检查 data 目录权限
ls -la data/

# 修复权限
sudo chown -R $USER:$USER data/
chmod -R 755 data/
```

#### C. 端口冲突
参考问题 1 的解决方案

---

### 5. 服务无法访问

**症状**: 部署成功但无法在浏览器中访问

**诊断步骤**:

```bash
# 1. 检查服务是否运行
docker compose ps

# 2. 检查端口是否正确
netstat -tlnp | grep -E "8080|3000|18080|13000"

# 3. 测试本地访问
curl http://localhost:18080/api/health
curl http://localhost:13000/health

# 4. 检查防火墙
sudo ufw status
sudo firewall-cmd --list-all
```

**解决方案**:

#### A. 开放防火墙端口

**Ubuntu/Debian (ufw)**:
```bash
sudo ufw allow 18080/tcp
sudo ufw allow 13000/tcp
sudo ufw reload
```

**CentOS/RHEL (firewalld)**:
```bash
sudo firewall-cmd --permanent --add-port=18080/tcp
sudo firewall-cmd --permanent --add-port=13000/tcp
sudo firewall-cmd --reload
```

**云服务器**: 在云服务商控制台配置安全组

#### B. 检查服务器 IP
```bash
# 获取公网 IP
curl ifconfig.me
curl icanhazip.com

# 获取内网 IP
hostname -I
ip addr show
```

---

### 6. 内存不足

**症状**:
```
Cannot allocate memory
OOMKilled
```

**解决方案**:

#### A. 增加 Swap 空间
```bash
# 创建 2GB swap 文件
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# 永久启用
echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

#### B. 限制 Docker 内存使用
```bash
# 编辑 /etc/docker/daemon.json
sudo nano /etc/docker/daemon.json
```

添加：
```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

#### C. 清理不必要的容器和镜像
```bash
# 清理停止的容器
docker container prune -f

# 清理未使用的镜像
docker image prune -a -f

# 清理构建缓存
docker builder prune -a -f
```

---

### 7. 构建超时

**症状**: 构建过程卡住或超时

**解决方案**:

```bash
# 1. 检查网络连接
ping -c 3 google.com
ping -c 3 github.com

# 2. 检查磁盘空间
df -h

# 3. 清理 Docker 缓存
docker system prune -a

# 4. 使用代理或镜像加速（参考问题 2）
```

---

### 8. 数据库锁定

**症状**:
```
database is locked
database table is locked
```

**解决方案**:

```bash
# 1. 停止服务
docker compose down

# 2. 检查是否有其他进程占用数据库
sudo lsof data/data.db

# 3. 删除锁文件（如果存在）
rm -f data/data.db-shm
rm -f data/data.db-wal

# 4. 重启服务
docker compose up -d
```

---

## 获取帮助

如果以上方案都无法解决您的问题：

1. **收集诊断信息**:
```bash
# 收集系统信息
./diagnose.sh > diagnostic-info.txt
```

2. **查看完整日志**:
```bash
docker compose logs --no-log-prefix > full-logs.txt
```

3. **联系支持**:
   - GitHub Issues: https://github.com/NoFxAiOS/nofx/issues
   - Telegram: https://t.me/nofx_dev_community

---

## 快速诊断脚本

创建 `diagnose.sh` 脚本：

```bash
#!/bin/bash

echo "=== NOFX 诊断信息 ==="
echo ""

echo "1. 系统信息:"
echo "   OS: $(cat /etc/os-release | grep PRETTY_NAME)"
echo "   Kernel: $(uname -r)"
echo "   内存: $(free -h | grep Mem | awk '{print $2}')"
echo "   磁盘: $(df -h / | tail -1 | awk '{print $4}') 可用"
echo ""

echo "2. Docker 版本:"
docker --version
docker compose version
echo ""

echo "3. Docker 状态:"
systemctl is-active docker
echo ""

echo "4. 容器状态:"
docker compose ps
echo ""

echo "5. 端口监听:"
netstat -tlnp | grep -E "8080|3000|18080|13000" || echo "   无端口监听"
echo ""

echo "6. 最近日志 (最后 20 行):"
docker compose logs --tail=20
echo ""

echo "7. 构建日志 (如果存在):"
if [ -f /tmp/nofx-build.log ]; then
    tail -20 /tmp/nofx-build.log
else
    echo "   无构建日志"
fi
```

使用方法：
```bash
chmod +x diagnose.sh
./diagnose.sh
```
