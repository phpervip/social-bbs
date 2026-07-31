# demo-e2e.ps1
# End-to-end verification against the B API Gateway (default http://localhost:8080).
# Compatible with Windows PowerShell 5.1+ and PowerShell 7. Output is ASCII-only.
# Steps: healthz -> dev login -> create post -> home timeline -> like -> comment
#        -> cross-user fanout -> delete authz/soft-delete.
# Exit code 0 = all steps PASS, 1 = any step FAIL.

param(
    [string]$BaseUrl = "http://localhost:8080"
)

$script:passCount = 0
$script:failCount = 0

function Write-Step {
    param([string]$Name, [bool]$Passed, [string]$Detail)
    if ($Passed) {
        $script:passCount++
        Write-Output ("[PASS] {0} - {1}" -f $Name, $Detail)
    }
    else {
        $script:failCount++
        Write-Output ("[FAIL] {0} - {1}" -f $Name, $Detail)
    }
}

# Invoke an HTTP call; never throws. Returns @{Ok; Status; Body; Error}.
# 4xx/5xx responses are captured via try/catch (PS 5.1 has no -SkipHttpErrorCheck).
function Invoke-Api {
    param([string]$Method, [string]$Path, [string]$Body, [string]$Token)
    $headers = @{ Accept = "application/json" }
    if ($Token) { $headers["Authorization"] = "Bearer $Token" }
    $params = @{
        Method     = $Method
        Uri        = ($BaseUrl + $Path)
        Headers    = $headers
        TimeoutSec = 15
    }
    if ($Body) {
        $params["ContentType"] = "application/json"
        $params["Body"] = $Body
    }
    try {
        $resp = Invoke-RestMethod @params
        return @{ Ok = $true; Status = 200; Body = $resp; Error = "" }
    }
    catch {
        $status = 0
        if ($_.Exception.Response) { $status = [int]$_.Exception.Response.StatusCode }
        return @{ Ok = $false; Status = $status; Body = $null; Error = $_.Exception.Message }
    }
}

Write-Output ("B demo e2e - target: {0}" -f $BaseUrl)
Write-Output "----------------------------------------"

# Step 1: health check
$r = Invoke-Api -Method GET -Path "/healthz"
Write-Step "healthz" ($r.Ok -and $r.Body.code -eq 0) `
    ("expected http 200 + code:0, got http=$($r.Status) code=$($r.Body.code)")

# Step 2: dev login as user 2 (alice), extract JWT
$r = Invoke-Api -Method POST -Path "/api/dev/login" -Body '{"user_id":2}'
$token = $r.Body.data.token
$dotCount = 0
if ($token) { $dotCount = ($token.ToCharArray() | Where-Object { $_ -eq "." }).Count }
Write-Step "login-user2" ($r.Ok -and $r.Body.code -eq 0 -and $dotCount -ge 2) `
    ("expected code:0 + JWT(2 dots), got code=$($r.Body.code) tokenDots=$dotCount http=$($r.Status)")
if (-not $token) {
    Write-Output "FATAL: cannot continue without a token."
    exit 1
}

# Step 3: create post
$stamp = Get-Date -Format "yyyyMMddHHmmss"
$content = "e2e demo post $stamp"
$body = @{ content = $content } | ConvertTo-Json -Compress
$r = Invoke-Api -Method POST -Path "/api/feed/post" -Body $body -Token $token
$postId = $r.Body.data.id
$createdAt = $r.Body.data.created_at
Write-Step "create-post" ($r.Ok -and $r.Body.code -eq 0 -and $postId -and $createdAt) `
    ("expected code:0 + data.id + data.created_at, got code=$($r.Body.code) id=$postId http=$($r.Status)")
if (-not $postId) {
    Write-Output "FATAL: cannot continue without a created post id."
    exit 1
}

# Step 4: home timeline contains the created post
$r = Invoke-Api -Method GET -Path "/api/feed/home?cursor=0&limit=20" -Token $token
$foundInHome = $false
if ($r.Ok -and $r.Body.code -eq 0 -and $r.Body.data.posts) {
    $foundInHome = ($r.Body.data.posts.id -contains $postId)
}
Write-Step "home-timeline" $foundInHome `
    ("expected code:0 + post id $postId in data.posts, got code=$($r.Body.code) found=$foundInHome http=$($r.Status)")

# Step 5: like the post, then verify liked_by_viewer + like_count
$likeBody = @{ post_id = $postId } | ConvertTo-Json -Compress
$r = Invoke-Api -Method POST -Path "/api/feed/like" -Body $likeBody -Token $token
$likeOk = ($r.Ok -and $r.Body.code -eq 0)
$r2 = Invoke-Api -Method GET -Path ("/api/feed/post/{0}" -f $postId) -Token $token
$viewerLiked = $r2.Body.data.liked_by_viewer
$likeCount = $r2.Body.data.like_count
$likeVerified = ($r2.Ok -and $viewerLiked -eq $true -and $likeCount -ge 1)
Write-Step "like" ($likeOk -and $likeVerified) `
    ("expected code:0 + liked_by_viewer=true + like_count>=1, got likeCode=$($r.Body.code) viewer=$viewerLiked likes=$likeCount")

# Step 6: comment, then verify it appears in the comment list
$cmtBody = @{ post_id = $postId; content = "e2e comment" } | ConvertTo-Json -Compress
$r = Invoke-Api -Method POST -Path "/api/feed/comment" -Body $cmtBody -Token $token
$cmtOk = ($r.Ok -and $r.Body.code -eq 0)
$r2 = Invoke-Api -Method GET -Path ("/api/feed/post/{0}/comments" -f $postId) -Token $token
$commentsList = $r2.Body.data
if ($commentsList.comments) { $commentsList = $commentsList.comments }
$commentFound = ($commentsList.content -contains "e2e comment")
Write-Step "comment" ($cmtOk -and $r2.Ok -and $r2.Body.code -eq 0 -and $commentFound) `
    ("expected code:0 + comment present, got cmtCode=$($r.Body.code) listCode=$($r2.Body.code) found=$commentFound")

# Step 7: global fanout - user 1 (bob) must see user 2's post in home timeline
$r = Invoke-Api -Method POST -Path "/api/dev/login" -Body '{"user_id":1}'
$token1 = $r.Body.data.token
$r2 = Invoke-Api -Method GET -Path "/api/feed/home?cursor=0&limit=20" -Token $token1
$fanoutFound = $false
if ($r2.Ok -and $r2.Body.code -eq 0 -and $r2.Body.data.posts) {
    $fanoutFound = ($r2.Body.data.posts.id -contains $postId)
}
Write-Step "fanout" ($r.Ok -and $r2.Ok -and $fanoutFound) `
    ("expected post id $postId in user1 timeline, got code=$($r2.Body.code) found=$fanoutFound")

# Step 8: delete - non-author forbidden(403), author ok(0), then 404
$r = Invoke-Api -Method DELETE -Path ("/api/feed/post/{0}" -f $postId) -Token $token1
$forbidden = (-not $r.Ok -and $r.Status -eq 403)
$r2 = Invoke-Api -Method DELETE -Path ("/api/feed/post/{0}" -f $postId) -Token $token
$deletedOk = ($r2.Ok -and $r2.Body.code -eq 0)
$r3 = Invoke-Api -Method GET -Path ("/api/feed/post/{0}" -f $postId) -Token $token
$gone = (-not $r3.Ok -and $r3.Status -eq 404)
Write-Step "delete" ($forbidden -and $deletedOk -and $gone) `
    ("expected 403(user1) + code:0(user2) + 404(after), got $($r.Status)/$($r2.Status)/$($r3.Status)")

Write-Output "----------------------------------------"
$total = $script:passCount + $script:failCount
Write-Output ("RESULT: {0}/{1} PASS, {2}/{1} FAIL" -f $script:passCount, $total, $script:failCount)
if ($script:failCount -eq 0) {
    Write-Output "E2E VERIFICATION SUCCEEDED"
    exit 0
}
else {
    Write-Output "E2E VERIFICATION FAILED"
    exit 1
}
