# start-all.ps1 — B (social-bbs) 本地一键启动脚本（Windows PowerShell 5.1+）
#
# 启动顺序（与根目录 README / infra/README 一致）：
#   1) docker compose up -d          （infra/，MySQL + Redis + MinIO）
#   2) 等待 MySQL / Redis 就绪        （docker exec mysqladmin ping / redis-cli ping 轮询）
#   3) Feed Service (Go)  :9000      （go run ./cmd/server）
#   4) Video Service (Go) :9002      （go run ./cmd/server）
#   5) Gateway (Node)     :8080      （npm run dev = nodemon src/server.js）
#   6) Shell (:3000) + Feed Remote (:3001) + User Remote (:3002)
#   7) Video Remote (:3003)
#
# 日志重定向到 $env:TEMP\b-logs\（本机 TEMP 为 D:\Personal\Temp\opencode 时即
# D:\Personal\Temp\opencode\b-logs），每个服务一对 .out.log / .err.log。
#
# 用法：
#   .\infra\start-all.ps1            # 启动全部
#   .\infra\start-all.ps1 -Stop      # 停止全部（按 PID 杀进程树 + docker compose down）
#
# 注意：Feed Service 的 config 是纯 os.Getenv（不读 .env 文件），
# 本脚本在启动前显式设置三个 FEED_* 环境变量（默认值与 internal/config 一致，可覆盖）。

#Requires -Version 5.1
[CmdletBinding()]
param(
    [switch]$Stop
)

$ErrorActionPreference = 'Stop'

# ---------------------------------------------------------------------------
# 配置
# ---------------------------------------------------------------------------
$ProjectRoot = Split-Path -Parent $PSScriptRoot          # 仓库根目录
$InfraDir    = $PSScriptRoot                              # infra/
$FeedDir     = Join-Path $ProjectRoot 'services\feed-service'
$UserDir     = Join-Path $ProjectRoot 'services\user-service'
$VideoDir    = Join-Path $ProjectRoot 'services\video-service'
$GatewayDir  = Join-Path $ProjectRoot 'services\gateway'
$ShellDir    = Join-Path $ProjectRoot 'frontend\shell'
$FeedRemoteDir  = Join-Path $ProjectRoot 'frontend\feed-remote'
$UserRemoteDir  = Join-Path $ProjectRoot 'frontend\user-remote'
$VideoRemoteDir = Join-Path $ProjectRoot 'frontend\video-remote'

$LogDir  = Join-Path $env:TEMP 'b-logs'
$PidFile = Join-Path $LogDir 'start-all.pids.json'        # PID 记录（-Stop 跨会话使用）

# Feed Service 环境变量（覆盖默认值时在调用前修改此处，或先在 shell 里 export）
$FeedGRPCAddr  = if ($env:FEED_GRPC_ADDR)  { $env:FEED_GRPC_ADDR }  else { ':9000' }
$FeedDBDsn     = if ($env:FEED_DB_DSN)     { $env:FEED_DB_DSN }     else { 'feed:feed123@tcp(127.0.0.1:3306)/feed_db?charset=utf8mb4&parseTime=True&loc=Local' }
$FeedRedisAddr = if ($env:FEED_REDIS_ADDR) { $env:FEED_REDIS_ADDR } else { 'localhost:6379' }

# Video Service 环境变量
$VideoGRPCAddr = if ($env:VIDEO_GRPC_ADDR) { $env:VIDEO_GRPC_ADDR } else { ':9002' }
$VideoDBDsn    = if ($env:VIDEO_DB_DSN)    { $env:VIDEO_DB_DSN }    else { 'video:video123@tcp(127.0.0.1:3306)/video_db?charset=utf8mb4&parseTime=True&loc=Local' }

# 健康检查超时（秒）
$MySqlTimeout    = 90
$RedisTimeout    = 60
$HttpTimeout     = 90

# ---------------------------------------------------------------------------
# 辅助函数
# ---------------------------------------------------------------------------
function Write-Step {
    param([string]$Message)
    Write-Host "`n==> $Message" -ForegroundColor Cyan
}

function Write-ErrorExit {
    param([string]$Message)
    Write-Host "ERROR: $Message" -ForegroundColor Red
    Write-Host 'Hint: 查看日志目录 ' -NoNewline
    Write-Host $LogDir -ForegroundColor Yellow
    exit 1
}

# 探测 go：先找 PATH，再退到常见安装路径（如 C:\Program Files\Go\bin\go.exe）
function Resolve-Go {
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }

    $candidates = @(
        (Join-Path $env:ProgramFiles 'Go\bin\go.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Go\bin\go.exe'),
        (Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe')
    )
    foreach ($c in $candidates) {
        if (Test-Path -LiteralPath $c) {
            Write-Host "   [go] 不在 PATH，使用探测到的完整路径: $c" -ForegroundColor Yellow
            return $c
        }
    }
    throw '未找到 go。请安装 Go 1.26+ 并加入 PATH（或确保 C:\Program Files\Go\bin\go.exe 存在）。'
}

# 轮询等待 docker 容器内 MySQL 就绪（init SQL 需要几秒）
function Wait-MySqlReady {
    $deadline = (Get-Date).AddSeconds($MySqlTimeout)
    while ((Get-Date) -lt $deadline) {
        $null = & docker exec b-mysql mysqladmin ping -uroot -proot123 --silent 2>$null
        if ($LASTEXITCODE -eq 0) { return $true }
        Start-Sleep -Seconds 3
    }
    return $false
}

# 轮询等待 Redis 就绪（redis-cli ping -> PONG）
function Wait-RedisReady {
    $deadline = (Get-Date).AddSeconds($RedisTimeout)
    while ((Get-Date) -lt $deadline) {
        $out = & docker exec b-redis redis-cli ping 2>$null
        if ($LASTEXITCODE -eq 0 -and $out -match 'PONG') { return $true }
        Start-Sleep -Seconds 3
    }
    return $false
}

# 轮询等待 HTTP 服务：任何 HTTP 响应（含 404/50x，如无 HtmlWebpackPlugin 的
# feed-remote 对 / 返回 404）都视为“服务在监听”；仅连接被拒时继续轮询。
function Wait-HttpUp {
    param([string]$Name, [string]$Url)
    $deadline = (Get-Date).AddSeconds($HttpTimeout)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
            Write-Host "   [$Name] 就绪 (HTTP $($resp.StatusCode)): $Url" -ForegroundColor Green
            return $true
        }
        catch {
            if ($_.Exception.Message -match '\(40\d\)|\(50\d\)') {
                Write-Host "   [$Name] 就绪 (HTTP 响应 $($_.Exception.Response.StatusCode.value__)): $Url" -ForegroundColor Green
                return $true
            }
            Start-Sleep -Seconds 2
        }
    }
    return $false
}

# 后台启动一个服务：返回 -PassThru 的 Process 对象
function Start-Bg {
    param(
        [string]$Name,
        [string]$FilePath,
        [string[]]$ArgumentList,
        [string]$WorkingDirectory
    )
    $outLog = Join-Path $LogDir "$Name.out.log"
    $errLog = Join-Path $LogDir "$Name.err.log"
    Write-Host "   [启动] $Name -> $FilePath $($ArgumentList -join ' ')  (cwd: $WorkingDirectory)" -ForegroundColor Gray
    return Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -WorkingDirectory $WorkingDirectory `
        -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden -PassThru
}

# 按 PID 杀进程树（go run / npm.cmd 会拉起子进程，必须 /T 连子进程一起杀）
function Stop-Tree {
    param([int]$Pid)
    if ($Pid -le 0) { return }
    try {
        & taskkill.exe /PID $Pid /T /F 2>$null | Out-Null
    }
    catch { } # 进程可能已退出，忽略
}

# 保存 PID 清单（供 -Stop 跨会话使用）
function Save-Pids {
    $pids = [ordered]@{
        feedService  = $FeedProc.Id
        videoService = $VideoProc.Id
        gateway      = $GatewayProc.Id
        shell        = $ShellProc.Id
        feedRemote   = $FeedRemoteProc.Id
        userRemote   = $UserRemoteProc.Id
        videoRemote  = $VideoRemoteProc.Id
    }
    $pids | ConvertTo-Json | Set-Content -LiteralPath $PidFile -Encoding UTF8
    Write-Host "   PID 清单已写入: $PidFile" -ForegroundColor Gray
}

# ---------------------------------------------------------------------------
# 停止模式：-Stop
# ---------------------------------------------------------------------------
if ($Stop) {
    Write-Step '停止全部服务（按 PID 杀进程树）'
    if (Test-Path -LiteralPath $PidFile) {
        $pids = Get-Content -LiteralPath $PidFile -Raw | ConvertFrom-Json
        foreach ($name in @('feedService', 'videoService', 'gateway', 'shell', 'feedRemote', 'userRemote', 'videoRemote')) {
            if ($pids.$name -and $pids.$name -gt 0) {
                Stop-Tree -Pid ([int]$pids.$name)
                Write-Host "   已停止 $name (PID $($pids.$name))"
            }
        }
        Remove-Item -LiteralPath $PidFile -Force
    }
    else {
        Write-Host '   未找到 PID 清单（可能尚未启动过）。' -ForegroundColor Yellow
    }

    Write-Step 'docker compose down（保留数据卷）'
    Push-Location $InfraDir
    try {
        & docker compose down
        if ($LASTEXITCODE -ne 0) { Write-Host '   docker compose down 返回非零退出码。' -ForegroundColor Yellow }
    }
    finally { Pop-Location }
    Write-Host "`n全部已停止。" -ForegroundColor Green
    exit 0
}

# ---------------------------------------------------------------------------
# 启动模式
# ---------------------------------------------------------------------------
Write-Step '前置检查'
# 确保日志目录存在
New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Write-Host "   日志目录: $LogDir"

# docker 可用
$null = & docker info 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-ErrorExit 'Docker 未运行或未安装。请先启动 Docker Desktop（docker info 应正常输出）。'
}

# node / npm 可用
$nodeCmd = Get-Command node -ErrorAction SilentlyContinue
if (-not $nodeCmd) { Write-ErrorExit '未找到 node。请安装 Node.js 18+。' }
Write-Host "   node: $($nodeCmd.Source)"
$npmCmd = Get-Command npm -ErrorAction SilentlyContinue
if (-not $npmCmd) { Write-ErrorExit '未找到 npm。请安装 Node.js 18+（含 npm）。' }

# go 可用（PATH 或常见安装路径）
try {
    $goExe = Resolve-Go
    Write-Host "   go: $goExe"
}
catch {
    Write-ErrorExit $_.Exception.Message
}

# 1) 基础设施
Write-Step '1/7 docker compose up -d（MySQL + Redis + MinIO）'
Push-Location $InfraDir
try {
    & docker compose up -d
    if ($LASTEXITCODE -ne 0) { Write-ErrorExit 'docker compose up -d 失败。' }
}
finally { Pop-Location }

# 2) 等待基础设施就绪
Write-Step '2/7 等待 MySQL / Redis 就绪'
if (-not (Wait-MySqlReady)) {
    Write-ErrorExit 'MySQL 在限时内未就绪。执行 `cd infra; docker compose ps` / `docker compose logs mysql` 排查。'
}
Write-Host '   [mysql] 就绪 (mysqld is alive)' -ForegroundColor Green
if (-not (Wait-RedisReady)) {
    Write-ErrorExit 'Redis 在限时内未就绪。执行 `cd infra; docker compose logs redis` 排查。'
}
Write-Host '   [redis] 就绪 (PONG)' -ForegroundColor Green

# 3) Feed Service（Go :9000）
Write-Step '3/7 Feed Service (Go, :9000)'
$env:FEED_GRPC_ADDR  = $FeedGRPCAddr
$env:FEED_DB_DSN     = $FeedDBDsn
$env:FEED_REDIS_ADDR = $FeedRedisAddr
$FeedProc = Start-Bg -Name 'feed-service' -FilePath $goExe -ArgumentList @('run', './cmd/server') -WorkingDirectory $FeedDir
Start-Sleep -Seconds 3 # 给 gRPC 监听一点启动时间（无 HTTP 健康端点，靠日志确认）

# 4) Video Service（Go :9002）
Write-Step '4/7 Video Service (Go, :9002)'
$env:VIDEO_GRPC_ADDR = $VideoGRPCAddr
$env:VIDEO_DB_DSN    = $VideoDBDsn
$VideoProc = Start-Bg -Name 'video-service' -FilePath $goExe -ArgumentList @('run', './cmd/server') -WorkingDirectory $VideoDir
Start-Sleep -Seconds 3

# 5) Gateway（Node :8080）
Write-Step '5/7 Gateway (Node, :8080)'
$GatewayProc = Start-Bg -Name 'gateway' -FilePath 'npm.cmd' -ArgumentList @('run', 'dev') -WorkingDirectory $GatewayDir
if (-not (Wait-HttpUp -Name 'gateway' -Url 'http://localhost:8080/healthz')) {
    Write-ErrorExit 'Gateway 健康检查失败（http://localhost:8080/healthz）。查看 gateway.err.log。'
}

# 6) 前端 Shell / Feed Remote / User Remote
Write-Step '6/7 Shell (:3000) + Feed Remote (:3001) + User Remote (:3002)'
$ShellProc = Start-Bg -Name 'shell' -FilePath 'npm.cmd' -ArgumentList @('run', 'dev') -WorkingDirectory $ShellDir
if (-not (Wait-HttpUp -Name 'shell' -Url 'http://localhost:3000')) {
    Write-ErrorExit 'Shell 启动失败（http://localhost:3000）。查看 shell.err.log。'
}
$FeedRemoteProc = Start-Bg -Name 'feed-remote' -FilePath 'npm.cmd' -ArgumentList @('run', 'dev') -WorkingDirectory $FeedRemoteDir
if (-not (Wait-HttpUp -Name 'feed-remote' -Url 'http://localhost:3001')) {
    Write-ErrorExit 'Feed Remote 启动失败（http://localhost:3001）。查看 feed-remote.err.log。'
}
$UserRemoteProc = Start-Bg -Name 'user-remote' -FilePath 'npm.cmd' -ArgumentList @('run', 'dev') -WorkingDirectory $UserRemoteDir
if (-not (Wait-HttpUp -Name 'user-remote' -Url 'http://localhost:3002')) {
    Write-ErrorExit 'User Remote 启动失败（http://localhost:3002）。查看 user-remote.err.log。'
}

# 7) Video Remote
Write-Step '7/7 Video Remote (:3003)'
$VideoRemoteProc = Start-Bg -Name 'video-remote' -FilePath 'npm.cmd' -ArgumentList @('run', 'dev') -WorkingDirectory $VideoRemoteDir
if (-not (Wait-HttpUp -Name 'video-remote' -Url 'http://localhost:3003')) {
    Write-ErrorExit 'Video Remote 启动失败（http://localhost:3003）。查看 video-remote.err.log。'
}

# 保存 PID + 输出汇总
Save-Pids

Write-Step '全部服务已启动'
Write-Host ''
Write-Host '  Service        PID    URL' -ForegroundColor White
Write-Host '  -------------- -----  -----------------------------' -ForegroundColor Gray
Write-Host ('  Shell          {0,-5}  http://localhost:3000' -f $ShellProc.Id)
Write-Host ('  Feed Remote    {0,-5}  http://localhost:3001' -f $FeedRemoteProc.Id)
Write-Host ('  User Remote    {0,-5}  http://localhost:3002' -f $UserRemoteProc.Id)
Write-Host ('  Video Remote   {0,-5}  http://localhost:3003' -f $VideoRemoteProc.Id)
Write-Host ('  Gateway        {0,-5}  http://localhost:8080/healthz' -f $GatewayProc.Id)
Write-Host ('  Feed Service   {0,-5}  :9000 (gRPC)' -f $FeedProc.Id)
Write-Host ('  Video Service  {0,-5}  :9002 (gRPC)' -f $VideoProc.Id)
Write-Host ''
Write-Host '  浏览器打开 http://localhost:3000 → 登录 → 发帖/上传视频' -ForegroundColor Cyan
Write-Host "  日志目录: $LogDir  |  停止: .\infra\start-all.ps1 -Stop" -ForegroundColor Gray
