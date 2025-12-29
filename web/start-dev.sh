#!/bin/bash
# NOFX Frontend Development Server Startup Script

echo "🚀 Starting NOFX Frontend Development Server..."
echo ""

# 进入 web 目录
cd "$(dirname "$0")"

# 检查 node_modules
if [ ! -d "node_modules" ]; then
    echo "📦 Installing dependencies..."
    npm install
fi

# 清除 Vite 缓存
echo "🧹 Clearing Vite cache..."
rm -rf node_modules/.vite
rm -rf .vite
rm -rf dist

# 启动开发服务器
echo "✅ Starting development server..."
echo ""
echo "══════════════════════════════════════════════════════════"
echo "  Frontend: http://localhost:3000"
echo "  Backend:  http://localhost:8080"
echo "══════════════════════════════════════════════════════════"
echo ""
echo "💡 Tip: Press Ctrl+C to stop the server"
echo "💡 Tip: Use 'Clear cache and hard reload' in browser if you don't see updates"
echo ""

npm run dev
