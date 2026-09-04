$ErrorActionPreference = 'Continue'
$base = 'C:\Users\zerof\blockads-windows-val\artifacts-tmp'
$log = Join-Path $base 'q1fail_L_O_prep.txt'
$exe = Join-Path $base 'blockads-service.exe'
$cli = Join-Path $base 'blockads-cli.exe'
$pd = Join-Path $env:PROGRAMDATA 'BlockAds'
$cfgPath = Join-Path $pd 'config.json'
$goodCatalog = 'https://raw.githubusercontent.com/pass-with-high-score/blockads-default-filter/refs/heads/main/output/filter_lists.json'
function L([string]$m) { Add-Content -Encoding utf8 $log $m }
function Write-Cfg($doh, $fb, $enabled) {
  $o = [ordered]@{
    version = 1; enabled = [bool]$enabled
    dns = [ordered]@{ listenPort = 53; protocol = 'doh'; primary = '1.1.1.1'; fallback = $fb; dohUrl = $doh }
    filters = [ordered]@{ catalogUrl = $goodCatalog; enabledListIds = @(); autoUpdate = $true }
  }
  [System.IO.File]::WriteAllText($cfgPath, ($o | ConvertTo-Json -Depth 6), (New-Object System.Text.UTF8Encoding $false))
}

Set-Content -Encoding utf8 $log '=== Q1FAIL + L + O PREP ==='
$env:Path = 'C:\Program Files\Go\bin;' + $env:Path
Set-Location 'C:\Users\zerof\blockads-windows-val\windows'
& go test ./... -count=1 > (Join-Path $base 'windows_tests3.txt') 2>&1
L ('windows_tests_exit=' + $LASTEXITCODE)
& go build -o $exe ./cmd/blockads-service
& go build -o $cli ./cmd/blockads-cli
Get-FileHash $exe -Algorithm SHA256 | ForEach-Object { L ('hash=' + $_.Hash) }

try { & $cli disable 2>$null | Out-Null } catch {}
try { & $exe stop 2>$null | Out-Null } catch {}
Start-Sleep 1
try { & $exe uninstall 2>$null | Out-Null } catch {}
Start-Sleep 1

L '=== DOH FAILURE (empty effective path) ==='
Write-Cfg 'https://127.0.0.1:1/dns-query' '203.0.113.50' $false
L ('disk_doh=' + ((Get-Content $cfgPath -Raw | ConvertFrom-Json).dns.dohUrl))
& $exe install $exe 2>&1 | ForEach-Object { L "$_" }
& $exe start 2>&1 | ForEach-Object { L "$_" }
Start-Sleep 2
& $cli enable 2>&1 | ForEach-Object { L ("enable=$_") }
Start-Sleep 1
& $cli status 2>&1 | ForEach-Object { L ("status=$_") }
$fail = (& nslookup example.com 127.0.0.1 2>&1 | Out-String) -replace "`r|`n",' | '
L ('nslookup_bad=' + $fail)
L 'policy=DoH then PLAIN fallback to 203.0.113.50 only; no system resolver'

L '=== RESTORE GOOD DOH + GATE L ==='
& $cli disable 2>&1 | Out-Null
& $exe stop 2>&1 | Out-Null; Start-Sleep 1
Write-Cfg 'https://cloudflare-dns.com/dns-query' '203.0.113.50' $false
& $exe start 2>&1 | Out-Null; Start-Sleep 2
& $cli enable 2>&1 | ForEach-Object { L ("enable_good=$_") }
Start-Sleep 1
$st0 = & $cli status 2>&1 | Out-String
$cb0=0; $re0=0
if ($st0 -match 'netwatchCallbacks":\s*(\d+)') { $cb0=[int64]$Matches[1] }
if ($st0 -match 'netwatchReevaluations":\s*(\d+)') { $re0=[int64]$Matches[1] }
L ('L_cb0=' + $cb0 + ' L_re0=' + $re0)
$eth = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.InterfaceDescription -match 'I225' } | Select-Object -First 1
L ('L_eth=' + $eth.Name + ' guid=' + $eth.InterfaceGuid)
if ($eth) {
  Disable-NetAdapter -Name $eth.Name -Confirm:$false
  Start-Sleep 2
  Enable-NetAdapter -Name $eth.Name -Confirm:$false
  $deadline = (Get-Date).AddSeconds(40)
  do { Start-Sleep 1 } while (((Get-NetAdapter -Name $eth.Name).Status -ne 'Up') -and ((Get-Date) -lt $deadline))
  Start-Sleep 3
  $st1 = & $cli status 2>&1 | Out-String
  $cb1=0; $re1=0
  if ($st1 -match 'netwatchCallbacks":\s*(\d+)') { $cb1=[int64]$Matches[1] }
  if ($st1 -match 'netwatchReevaluations":\s*(\d+)') { $re1=[int64]$Matches[1] }
  L ('L_cb1=' + $cb1 + ' L_re1=' + $re1)
  L ('L_cb_delta=' + ($cb1 - $cb0) + ' L_re_delta=' + ($re1 - $re0))
  Get-DnsClientServerAddress -InterfaceAlias $eth.Name -AddressFamily IPv4 -EA SilentlyContinue | ForEach-Object { L ('L_dns=' + (($_.ServerAddresses) -join ',')) }
  $owned = ($st1 -match '"owned": true')
  L ('L_status_owned=' + $owned)
  & $cli disable 2>&1 | ForEach-Object { L ("L_disable=$_") }
  Start-Sleep 1
  Get-DnsClientServerAddress -InterfaceAlias $eth.Name -AddressFamily IPv4 -EA SilentlyContinue | ForEach-Object { L ('L_dns_restored=' + (($_.ServerAddresses) -join ',')) }
}

L '=== O1 PREP: enable protection for reboot ==='
Write-Cfg 'https://cloudflare-dns.com/dns-query' '203.0.113.50' $true
& $exe stop 2>&1 | Out-Null; Start-Sleep 1; & $exe start 2>&1 | Out-Null; Start-Sleep 3
& $cli status 2>&1 | ForEach-Object { L ("O1_pre_status=$_") }
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | ForEach-Object { L ('O1_pre_dns=' + $_.InterfaceAlias + '|' + ($_.ServerAddresses -join ',')) }
$pre = Join-Path $base 'reboot_o1_pre.json'
& $cli status 2>&1 | Out-File -Encoding utf8 $pre
L ('O1_pre_saved=' + $pre)
L '=== DONE (ready for reboot if O1_pre shows filteringEnabled true) ==='
