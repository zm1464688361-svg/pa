# 端到端验收脚本：按题面场景走完四角色主流程，输出记录到 docs/verification.md
# 前置：service(9001)/gateway(8081)/paysim(9002) 已启动；MySQL 可用；已有 seed 数据
# 数据库口令不落库：优先读环境变量 MYSQL_USER / MYSQL_PWD（mysql 客户端自动识别 MYSQL_PWD），
# 其次从 config.local.yaml 的 db_dsn 解析；两者都没有则直接报错退出，禁止在脚本里写死口令。
$ErrorActionPreference = "Continue"
$GW = "http://127.0.0.1:8081"
$PS = "http://127.0.0.1:9002"
$MYSQL = "C:\Program Files\MySQL\MySQL Server 8.0\bin\mysql.exe"

$MYSQL_USER = if ($env:MYSQL_USER) { $env:MYSQL_USER } else { "root" }
if (-not $env:MYSQL_PWD) {
    $cfg = Join-Path (Split-Path -Parent $PSScriptRoot) "config.local.yaml"
    if (Test-Path $cfg) {
        $line = (Select-String -Path $cfg -Pattern "^\s*db_dsn\s*:" | Select-Object -First 1).Line
        if ($line -match "^\s*db_dsn\s*:\s*[`"']?([^:@`"'\s]+):([^@`"']*)@") {
            $MYSQL_USER = $Matches[1]
            $env:MYSQL_PWD = $Matches[2]
        }
    }
}
if (-not $env:MYSQL_PWD) {
    throw "未提供数据库口令：请先设置环境变量 MYSQL_PWD（或 MYSQL_USER/MYSQL_PWD），或准备 config.local.yaml 的 db_dsn。脚本不再内置口令。"
}

$script:rows = @()
function Step($name, $ok, $detail) {
    $r = "| $name | " + $(if ($ok) { "通过" } else { "失败" }) + " | $detail |"
    $script:rows += $r
    if ($ok) { Write-Host "OK   $name" } else { Write-Host "FAIL $name -> $detail" }
}
function SQL($q) { (& $MYSQL "-u$MYSQL_USER" petshop -N -B -e $q 2>$null) -join "`n" }
function Login($u) {
    $s = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    Invoke-WebRequest -Uri "$GW/login" -Method Post -Body @{ username = $u } -WebSession $s -UseBasicParsing | Out-Null
    return $s
}
function Post($s, $path, $body) {
    return Invoke-WebRequest -Uri "$GW$path" -Method Post -Body $body -WebSession $s -UseBasicParsing -MaximumRedirection 5
}

$owner = Login "owner"; $u1 = Login "user1"; $u2 = Login "user2"; $front = Login "front"; $support = Login "support"
$pet1 = SQL "SELECT id FROM pets WHERE user_id=(SELECT id FROM users WHERE username='user1') LIMIT 1;"
$pet2 = SQL "SELECT id FROM pets WHERE user_id=(SELECT id FROM users WHERE username='user2') LIMIT 1;"
Step "四角色登录" ($pet1 -and $pet2) "user1 宠物=$pet1，user2 宠物=$pet2"

function NewSlot($hoursAhead, $cap) {
    $st = (Get-Date).AddHours($hoursAhead).ToString("yyyy-MM-dd HH:mm")
    $en = (Get-Date).AddHours($hoursAhead + 1).ToString("yyyy-MM-dd HH:mm")
    Post $owner "/owner/slots" @{ start_time = $st; end_time = $en; capacity = $cap } | Out-Null
    return SQL "SELECT id FROM time_slots WHERE start_time=STR_TO_DATE('$st','%Y-%m-%d %H:%i') ORDER BY id DESC LIMIT 1;"
}

# ---------- 场景 1：≥2h 自助取消 + 退款成功释放名额（幂等） ----------
$slotA = NewSlot 30 3
Post $u1 "/bookings" @{ slot_id = $slotA; pet_id = $pet1 } | Out-Null
$apA = SQL "SELECT id FROM appointments WHERE slot_id=$slotA ORDER BY id DESC LIMIT 1;"
$orderA = SQL "SELECT order_no FROM appointments WHERE id=$apA;"
$remA = SQL "SELECT remaining FROM time_slots WHERE id=$slotA;"
Step "提交预约预扣名额" ($remA -eq "2") "预约=$apA 订单=$orderA 余量=$remA"

Post $u1 "/pay/$orderA" @{ result = "success" } | Out-Null
Start-Sleep 1
$stA = SQL "SELECT status FROM appointments WHERE id=$apA;"
Step "支付回调确认预约" ($stA -eq "confirmed") "状态=$stA"

Post $u1 "/bookings/$apA/cancel" @{ reason = "临时有事" } | Out-Null
Start-Sleep 1
$stA2 = SQL "SELECT status FROM appointments WHERE id=$apA;"
$rfA = SQL "SELECT refund_no FROM refunds WHERE booking_id=$apA ORDER BY id DESC LIMIT 1;"
$remA2 = SQL "SELECT remaining FROM time_slots WHERE id=$slotA;"
Step "距开始≥2h 自助取消" ($stA2 -eq "cancelled" -and $rfA) "状态=$stA2 退款单=$rfA 名额仍锁定=$remA2"

Post $support "/support/refunds/$rfA/retry" @{} -ErrorAction SilentlyContinue | Out-Null
Invoke-WebRequest -Uri "$PS/action/refund" -Method Post -Body @{ refund_no = $rfA; result = "success" } -UseBasicParsing | Out-Null
Start-Sleep 2
$rstA = SQL "SELECT status FROM refunds WHERE refund_no='$rfA';"
$remA3 = SQL "SELECT remaining FROM time_slots WHERE id=$slotA;"
$relA = SQL "SELECT slot_released FROM appointments WHERE id=$apA;"
Step "退款成功原路返还名额" ($rstA -eq "success" -and $remA3 -eq "3") "退款=$rstA 名额=$remA3 已释放标记=$relA"

Invoke-WebRequest -Uri "$PS/action/refund" -Method Post -Body @{ refund_no = $rfA; result = "success" } -UseBasicParsing | Out-Null
Start-Sleep 2
$remA4 = SQL "SELECT remaining FROM time_slots WHERE id=$slotA;"
Step "重复退款回调名额幂等" ($remA4 -eq "3") "名额=$remA4（不得二次释放）"

# ---------- 场景 2：<2h 人工取消审核 → 批准退款 ----------
$slotB = NewSlot 1 3
Post $u2 "/bookings" @{ slot_id = $slotB; pet_id = $pet2 } | Out-Null
$apB = SQL "SELECT id FROM appointments WHERE slot_id=$slotB ORDER BY id DESC LIMIT 1;"
$orderB = SQL "SELECT order_no FROM appointments WHERE id=$apB;"
Post $u2 "/pay/$orderB" @{ result = "success" } | Out-Null
Start-Sleep 1
$noReason = Post $u2 "/bookings/$apB/cancel" @{ reason = "" } -ErrorAction SilentlyContinue
$hasErr = ($noReason.Content -match "原因")
Step "空取消原因被拒绝" $hasErr "页面提示原因必填=$hasErr"

Post $u2 "/bookings/$apB/cancel" @{ reason = "临时出差" } | Out-Null
$reqB = SQL "SELECT id FROM cancel_requests WHERE booking_id=$apB AND status='pending' LIMIT 1;"
$ciB = ""
try { $ciB = (Post $front "/front/$apB/checkin" @{} -ErrorAction SilentlyContinue).Content } catch { $ciB = "" }
$blocked = ($ciB -match "审核中|拒绝|不可")
Step "审核期间拦截核销" ($reqB -and $blocked) "申请=$reqB 核销被拦截=$blocked"

Post $support "/support/review" @{ request_id = $reqB; decision = "approve"; reason = "情况属实" } | Out-Null
Start-Sleep 1
$rfB = SQL "SELECT refund_no FROM refunds WHERE booking_id=$apB LIMIT 1;"
$stB = SQL "SELECT status FROM appointments WHERE id=$apB;"
Step "客服批准人工取消" ($stB -eq "cancelled" -and $rfB) "状态=$stB 退款单=$rfB"

Invoke-WebRequest -Uri "$PS/action/refund" -Method Post -Body @{ refund_no = $rfB; result = "success" } -UseBasicParsing | Out-Null
Start-Sleep 2
$remB = SQL "SELECT remaining FROM time_slots WHERE id=$slotB;"
Step "人工取消退款成功释放名额" ($remB -eq "3") "名额=$remB"

# ---------- 场景 3：核销 → 服务开始 → 完成（幂等、禁止跳步） ----------
$slotC = NewSlot 26 3
Post $u1 "/bookings" @{ slot_id = $slotC; pet_id = $pet1 } | Out-Null
$apC = SQL "SELECT id FROM appointments WHERE slot_id=$slotC ORDER BY id DESC LIMIT 1;"
$orderC = SQL "SELECT order_no FROM appointments WHERE id=$apC;"
Post $u1 "/pay/$orderC" @{ result = "success" } | Out-Null
Start-Sleep 1
$c1 = (Post $front "/front/$apC/checkin" @{ }).Content
$stC1 = SQL "SELECT status FROM appointments WHERE id=$apC;"
$c2 = (Post $front "/front/$apC/checkin" @{ }).Content
$cntC = SQL "SELECT COUNT(*) FROM audit_logs WHERE action='check_in' AND target_id=$apC;"
Step "核销并幂等" ($stC1 -eq "checked_in" -and $cntC -eq "1") "状态=$stC1 核销流水数=$cntC"

$skip = ""
try { $skip = (Post $front "/front/$apC/complete" @{}).Content } catch { $skip = "" }
$skippedOK = ($skip -match "开始|顺序|失败")
Post $front "/front/$apC/start" @{} | Out-Null
Post $front "/front/$apC/complete" @{} | Out-Null
$stC2 = SQL "SELECT status FROM appointments WHERE id=$apC;"
Step "禁止跳步且完成服务" ($skippedOK -and $stC2 -eq "completed") "跳步被拒=$skippedOK 终态=$stC2"

# ---------- 场景 4：关单后迟到支付 → 异常支付台账 → 客服退款 ----------
$slotD = NewSlot 28 3
Post $u2 "/bookings" @{ slot_id = $slotD; pet_id = $pet2 } | Out-Null
$apD = SQL "SELECT id FROM appointments WHERE slot_id=$slotD ORDER BY id DESC LIMIT 1;"
$orderD = SQL "SELECT order_no FROM appointments WHERE id=$apD;"
Post $u2 "/bookings/$apD/cancel-unpaid" @{} | Out-Null
$stD1 = SQL "SELECT status FROM appointments WHERE id=$apD;"
Post $u2 "/pay/$orderD" @{ result = "success" } | Out-Null
Start-Sleep 2
$abD = SQL "SELECT COUNT(*) FROM abnormal_payments WHERE order_no='$orderD';"
$stD2 = SQL "SELECT status FROM appointments WHERE id=$apD;"
Step "关单后支付进入异常台账" ($stD1 -eq "closed" -and $abD -eq "1" -and $stD2 -eq "closed") "关单=$stD1 异常条数=$abD 预约状态不变=$stD2"

$abID = SQL "SELECT id FROM abnormal_payments WHERE order_no='$orderD' LIMIT 1;"
Post $support "/support/abnormal/$abID/refund" @{} | Out-Null
$rfD = SQL "SELECT refund_no FROM refunds WHERE abnormal_id=$abID LIMIT 1;"
Invoke-WebRequest -Uri "$PS/action/refund" -Method Post -Body @{ refund_no = $rfD; result = "success" } -UseBasicParsing | Out-Null
Start-Sleep 2
$abD2 = SQL "SELECT status FROM abnormal_payments WHERE id=$abID;"
$remD = SQL "SELECT remaining FROM time_slots WHERE id=$slotD;"
$relD = SQL "SELECT slot_released FROM appointments WHERE id=$apD;"
Step "异常支付退款不释放名额" ($abD2 -eq "refunded" -and $remD -eq "3" -and $relD -eq "0") "台账=$abD2 名额=$remD（关单时已释放，异常退款不再变动）释放标记=$relD"

# ---------- 场景 5：未到店标记（时段结束后） ----------
$slotE = NewSlot 2 3
Post $u1 "/bookings" @{ slot_id = $slotE; pet_id = $pet1 } | Out-Null
$apE = SQL "SELECT id FROM appointments WHERE slot_id=$slotE ORDER BY id DESC LIMIT 1;"
$orderE = SQL "SELECT order_no FROM appointments WHERE id=$apE;"
Post $u1 "/pay/$orderE" @{ result = "success" } | Out-Null
Start-Sleep 1
$early = ""
try { $early = (Post $owner "/owner/noshow" @{ booking_id = $apE; reason = "未到" }).Content } catch { $early = "" }
$earlyBlocked = ($early -match "未到达|不能标记|结束")
Post $owner "/clock" @{ offset_sec = "14400" } | Out-Null
Start-Sleep 1
Post $owner "/owner/noshow" @{ booking_id = $apE; reason = "未到店且未接电话" } | Out-Null
Start-Sleep 1
$stE = SQL "SELECT status FROM appointments WHERE id=$apE;"
$remE = SQL "SELECT remaining FROM time_slots WHERE id=$slotE;"
Step "未到店需时段结束后才能标记" ($earlyBlocked -and $stE -eq "no_show" -and $remE -eq "2") "提前标记被拒=$earlyBlocked 状态=$stE 名额不返还=$remE"
Post $owner "/clock" @{ offset_sec = "0" } | Out-Null

# ---------- 场景 6：店长日清报表 ----------
$day = (Get-Date).AddHours(2).ToString("yyyy-MM-dd")
$rep = (Invoke-WebRequest -Uri "$GW/owner?date=$day" -WebSession $owner -UseBasicParsing).Content
$hasReport = ($rep -match "已收款|实收|已退款")
$apiDenied = ""
try { $apiDenied = (Invoke-WebRequest -Uri "$GW/owner?date=$day" -WebSession $u1 -UseBasicParsing).Content } catch { $apiDenied = "denied" }
$deniedOK = ($apiDenied -match "无权|权限|仅店长|denied")
Step "店长可看日清报表且主人无权" ($hasReport -and $deniedOK) "报表渲染=$hasReport 主人被拒=$deniedOK"

# ---------- 输出记录 ----------
$head = @"
# 端到端验收记录（自动生成）

- 生成时间：$(Get-Date -Format "yyyy-MM-dd HH:mm:ss")
- 环境：本机 Windows + MySQL 8.0（库 petshop）+ service:9001 / gateway:8081 / paysim:9002
- 脚本：``scripts/e2e.ps1``（可重复执行，但会持续产生演示数据）

| 场景 | 结果 | 关键数据 |
|---|---|---|
"@
$tail = @"

## 统计口径说明

- 场景 1 验证「退款成功才释放名额」与「重复回调幂等」。
- 场景 2 验证「<2h 必须人工审核」「审核期间禁止核销」。
- 场景 3 验证「核销→开始→完成强制顺序」「重复核销不产生新流水」。
- 场景 4 验证「关单后支付进入异常台账」「异常退款不释放名额、不恢复预约」。
- 场景 5 验证「未到店必须时段结束后由店长手动标记」「名额不返还」。
- 场景 6 验证「店长可见日清报表、主人无权访问」。
"@
($head + ($script:rows -join "`n") + $tail) | Out-File -FilePath "docs/verification.md" -Encoding utf8
Write-Host "已写入 docs/verification.md"
