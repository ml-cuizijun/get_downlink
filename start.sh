#!/bin/bash
APP_NAME="bot-linux"
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$APP_DIR/$APP_NAME.pid"
LOG_FILE="$APP_DIR/$APP_NAME.log"

start() {
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "⚠️  $APP_NAME 已在运行 (PID: $(cat "$PID_FILE"))"
        return 1
    fi
    echo "🚀 启动 $APP_NAME ..."
    nohup "$APP_DIR/$APP_NAME" >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    echo "✅ 已启动 (PID: $!), 日志: $LOG_FILE"
}

stop() {
    if [ ! -f "$PID_FILE" ]; then
        echo "⚠️  PID 文件不存在，$APP_NAME 未运行"
        return 1
    fi
    local pid
    pid=$(cat "$PID_FILE")
    if kill -0 "$pid" 2>/dev/null; then
        echo "🛑 停止 $APP_NAME (PID: $pid) ..."
        kill "$pid"
        sleep 2
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid"
        fi
        echo "✅ 已停止"
    else
        echo "⚠️  进程 $pid 不存在"
    fi
    rm -f "$PID_FILE"
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "✅ $APP_NAME 运行中 (PID: $(cat "$PID_FILE"))"
    else
        echo "❌ $APP_NAME 未运行"
        rm -f "$PID_FILE"
    fi
}

case "${1:-start}" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; sleep 1; start ;;
    status)  status ;;
    *)       echo "用法: $0 {start|stop|restart|status}" ;;
esac
