# P2 Demo Script — 账号体系 + 关注 + Kafka 事件化验证
# 运行前确保：MySQL/Redis/Kafka/Feed Service/User Service/Gateway 均已启动
# 用法：.\demo-p2.ps1

$base = "http://localhost:8080"
$pass = "Password123!"

Write-Host "=== P2 Demo: 账号体系 + 关注 + Kafka 事件化 ===" -ForegroundColor Cyan
Write-Host ""

# 1. Health check
Write-Host "1. Health Check" -ForegroundColor Yellow
try {
    $h = Invoke-RestMethod -Uri "$base/healthz" -TimeoutSec 5
    Write-Host "   Gateway: $($h.message)" -ForegroundColor Green
} catch {
    Write-Host "   Gateway 不可用: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}

# 2. Register alice
Write-Host "2. Register alice" -ForegroundColor Yellow
$aliceBody = @{username="demo_alice";email="demo_alice@test.com";password=$pass;display_name="Demo Alice"} | ConvertTo-Json
$alice = Invoke-RestMethod -Uri "$base/api/auth/register" -Method Post -Body $aliceBody -ContentType "application/json" -TimeoutSec 15
$aTok = $alice.data.token
$aUid = $alice.data.user.id
Write-Host "   alice id=$aUid, token=$($aTok.Substring(0,30))..." -ForegroundColor Green

# 3. Register bob
Write-Host "3. Register bob" -ForegroundColor Yellow
$bobBody = @{username="demo_bob";email="demo_bob@test.com";password=$pass;display_name="Demo Bob"} | ConvertTo-Json
$bob = Invoke-RestMethod -Uri "$base/api/auth/register" -Method Post -Body $bobBody -ContentType "application/json" -TimeoutSec 15
$bTok = $bob.data.token
$bUid = $bob.data.user.id
Write-Host "   bob id=$bUid" -ForegroundColor Green

# 4. Alice follows bob
Write-Host "4. Alice follows bob" -ForegroundColor Yellow
$empty = '{}'
$fol = Invoke-WebRequest -Uri "$base/api/user/$bUid/follow" -Method Post -Body $empty -ContentType "application/json" -Headers @{Authorization="Bearer $aTok"} -TimeoutSec 15
Write-Host "   Follow: $($fol.StatusCode)" -ForegroundColor Green

# 5. Bob posts 3 posts
Write-Host "5. Bob posts 3 posts" -ForegroundColor Yellow
1..3 | ForEach-Object {
    $pBody = @{content="Demo post #$_ at $(Get-Date -Format HH:mm:ss)"} | ConvertTo-Json
    $p = Invoke-WebRequest -Uri "$base/api/feed/post" -Method Post -Body $pBody -ContentType "application/json" -Headers @{Authorization="Bearer $bTok"} -TimeoutSec 15
    Write-Host "   post #$_: $(($p.Content | ConvertFrom-Json).data.id)" -ForegroundColor Green
}

# 6. Alice home timeline (should see bob's posts)
Write-Host "6. Alice home timeline (should see 3 bob posts)" -ForegroundColor Yellow
Start-Sleep -Seconds 1
$home = Invoke-RestMethod -Uri "$base/api/feed/home?cursor=0&limit=10" -Headers @{Authorization="Bearer $aTok"} -TimeoutSec 15
Write-Host "   posts count=$($home.data.posts.Count)" -ForegroundColor Green
$home.data.posts | ForEach-Object { Write-Host "   - [$($_.user_id)] $($_.content)" }

# 7. Alice unfollows bob
Write-Host "7. Alice unfollows bob" -ForegroundColor Yellow
$unfol = Invoke-WebRequest -Uri "$base/api/user/$bUid/follow" -Method Delete -Body $empty -ContentType "application/json" -Headers @{Authorization="Bearer $aTok"} -TimeoutSec 15
Write-Host "   Unfollow: $($unfol.StatusCode)" -ForegroundColor Green

# 8. Bob posts a new post
Write-Host "8. Bob posts AFTER unfollow" -ForegroundColor Yellow
$pBody = @{content="Bob AFTER unfollow at $(Get-Date -Format HH:mm:ss)"} | ConvertTo-Json
$p = Invoke-WebRequest -Uri "$base/api/feed/post" -Method Post -Body $pBody -ContentType "application/json" -Headers @{Authorization="Bearer $bTok"} -TimeoutSec 15
$newId = ($p.Content | ConvertFrom-Json).data.id
Write-Host "   new post id=$newId" -ForegroundColor Green

# 9. Alice home (should NOT see new post)
Write-Host "9. Alice home after unfollow (should NOT see new post)" -ForegroundColor Yellow
Start-Sleep -Seconds 1
$home2 = Invoke-RestMethod -Uri "$base/api/feed/home?cursor=0&limit=10" -Headers @{Authorization="Bearer $aTok"} -TimeoutSec 15
$newPostVisible = $home2.data.posts | Where-Object { $_.id -eq $newId }
if ($newPostVisible) {
    Write-Host "   FAIL: New post visible after unfollow!" -ForegroundColor Red
} else {
    Write-Host "   PASS: New post NOT visible after unfollow" -ForegroundColor Green
}

# 10. Check Kafka outbox
Write-Host "10. Kafka outbox verification" -ForegroundColor Yellow
rtk docker exec b-mysql mysql -u root -proot123 feed_db -e "SELECT COUNT(*) as total, SUM(status='delivered') as delivered FROM outbox_events;" 2>$null
rtk docker exec b-mysql mysql -u root -proot123 user_db -e "SELECT COUNT(*) as total, SUM(status='delivered') as delivered FROM user_outbox;" 2>$null

Write-Host ""
Write-Host "=== P2 Demo Complete ===" -ForegroundColor Cyan
