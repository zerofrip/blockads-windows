$ErrorActionPreference = 'Continue'
$base = 'C:\Users\zerof\blockads-windows-val\artifacts-tmp'
$log = Join-Path $base 'q123_validation.txt'
$exe = Join-Path $base 'blockads-service.exe'
$cli = Join-Path $base 'blockads-cli.exe'
$pd = Join-Path $env:PROGRAMDATA 'BlockAds'
$cfgPath = Join-Path $pd 'config.json'
$goodCatalog = 'https://raw.githubusercontent.com/pass-with-high-score/blockads-default-filter/refs/heads/main/output/filter_lists.json'
function L([string]$m) { Add-Content -Encoding utf8 $log $m }

Set-Content -Encoding utf8 $log '=== Q1 Q2 Q3 FILTER-FAIL ==='
$env:Path = 'C:\Program Files\Go\bin;' + $env:Path
Set-Location 'C:\Users\zerof\blockads-windows-val\tunnel'
& go test ./... -count=1 > (Join-Path $base 'tunnel_tests.txt') 2>&1
L ('tunnel_tests_exit=' + $LASTEXITCODE)
Set-Location 'C:\Users\zerof\blockads-windows-val\windows'
& go test ./... -count=1 > (Join-Path $base 'windows_tests.txt') 2>&1
L ('windows_tests_exit=' + $LASTEXITCODE)
& go build -o $exe ./cmd/blockads-service
if ($LASTEXITCODE -ne 0) { L 'BUILD_SERVICE_FAILED'; exit 1 }
& go build -o $cli ./cmd/blockads-cli
if ($LASTEXITCODE -ne 0) { L 'BUILD_CLI_FAILED'; exit 1 }
Get-FileHash $exe,$cli -Algorithm SHA256 | ForEach-Object { L ('hash=' + $_.Hash) }

function Ensure-Service {
  param([bool]$WantEnabledConfig = $false, [string]$DohUrl = 'https://cloudflare-dns.com/dns-query', [string]$Fallback = '203.0.113.50', [string]$Catalog = $goodCatalog)
  New-Item -ItemType Directory -Force -Path $pd | Out-Null
  try { & $cli disable 2>$null | Out-Null } catch {}
  try { & $exe stop 2>$null | Out-Null } catch {}
  Start-Sleep 1
  $o = [ordered]@{
    version = 1
    enabled = [bool]$WantEnabledConfig
    dns = [ordered]@{ listenPort = 53; protocol = 'doh'; primary = '1.1.1.1'; fallback = $Fallback; dohUrl = $DohUrl }
    filters = [ordered]@{ catalogUrl = $Catalog; enabledListIds = @(); autoUpdate = $true }
  }
  [System.IO.File]::WriteAllText($cfgPath, ($o | ConvertTo-Json -Depth 6), (New-Object System.Text.UTF8Encoding $false))
  L ('cfg_protocol=' + ((Get-Content $cfgPath -Raw | ConvertFrom-Json).dns.protocol))
  L ('cfg_doh=' + ((Get-Content $cfgPath -Raw | ConvertFrom-Json).dns.dohUrl))
  $st = & $exe status 2>$null
  if ("$st" -match 'not_installed' -or $LASTEXITCODE -ne 0) {
    & $exe install $exe 2>&1 | ForEach-Object { L ("install=$_") }
  }
  & $exe start 2>&1 | ForEach-Object { L ("start=$_") }
  for ($i=0; $i -lt 40; $i++) {
    if ((Get-Service BlockAdsService -EA SilentlyContinue).Status -eq 'Running') { break }
    Start-Sleep -Milliseconds 250
  }
  L ('svc=' + (Get-Service BlockAdsService -EA SilentlyContinue).Status)
}

try { & $exe uninstall 2>$null | Out-Null } catch {}
Start-Sleep 1
Ensure-Service -WantEnabledConfig $false

L '=== Q1 DOH ==='
& $cli enable 2>&1 | ForEach-Object { L ("enable=$_") }
Start-Sleep 2
$proc = Get-Process blockads-service -EA SilentlyContinue | Select-Object -First 1
L ('pid=' + $proc.Id)
& $cli status 2>&1 | ForEach-Object { L ("status=$_") }
$before = @(Get-NetTCPConnection -OwningProcess $proc.Id -RemotePort 443 -EA SilentlyContinue)
L ('tcp443_before=' + $before.Count)
Resolve-DnsName example.com -Type A -DnsOnly -EA SilentlyContinue | Select-Object -First 2 | ForEach-Object { L ('resolve=' + $_.IPAddress) }
Start-Sleep 1
$after = @(Get-NetTCPConnection -OwningProcess $proc.Id -RemotePort 443 -EA SilentlyContinue)
L ('tcp443_after=' + $after.Count)
$after | Select-Object -First 8 | ForEach-Object { L ('tcp443=' + $_.RemoteAddress + ':' + $_.RemotePort + '/' + $_.State) }
# Resolve cloudflare-dns.com to compare
try {
  $dohIps = [System.Net.Dns]::GetHostAddresses('cloudflare-dns.com') | ForEach-Object { $_.IPAddressToString }
  L ('doh_endpoint_ips=' + ($dohIps -join ','))
  $match = @($after | Where-Object { $dohIps -contains $_.RemoteAddress.IPAddressToString -or $dohIps -contains ([string]$_.RemoteAddress) })
  L ('tcp443_to_doh_endpoint=' + $match.Count)
  $match | ForEach-Object { L ('doh_conn=' + $_.RemoteAddress) }
} catch { L ('doh_ip_err=' + $_.Exception.Message) }

L '=== Q1 DOH FAIL ==='
& $cli disable 2>&1 | Out-Null
Ensure-Service -WantEnabledConfig $false -DohUrl 'https://127.0.0.1:1/dns-query' -Fallback '203.0.113.50'
& $cli enable 2>&1 | ForEach-Object { L ("enable_bad=$_") }
Start-Sleep 1
$fail = (& nslookup example.com 127.0.0.1 2>&1 | Out-String) -replace "`r|`n",' | '
L ('nslookup_bad=' + $fail)
L 'fallback_policy=DoH then configured PLAIN fallback only; no system resolver'

L '=== RESTORE ==='
& $cli disable 2>&1 | Out-Null
Ensure-Service -WantEnabledConfig $false
& $cli enable 2>&1 | ForEach-Object { L ("enable_ok=$_") }
Start-Sleep 1

L '=== N1 ==='
$jobs = 1..20 | ForEach-Object { Start-Job { param($c) try { & $c status 2>&1 | Out-String } catch { $_.Exception.Message } } -ArgumentList $cli }
$outs = $jobs | Wait-Job -Timeout 60 | Receive-Job
$jobs | Remove-Job -Force
L ('N1_ok=' + (@($outs | Where-Object { $_ -match 'filteringEnabled' }).Count) + '/20')

L '=== N2 ==='
$j1 = Start-Job { param($c) & $c enable 2>&1 | Out-String } -ArgumentList $cli
$j2 = Start-Job { param($c) & $c enable 2>&1 | Out-String } -ArgumentList $cli
Wait-Job $j1,$j2 -Timeout 60 | Out-Null
L ('N2a=' + ((Receive-Job $j1) -replace "`r|`n",' '))
L ('N2b=' + ((Receive-Job $j2) -replace "`r|`n",' '))
Remove-Job $j1,$j2 -Force
$st = & $cli status 2>&1 | Out-String
L ('N2_filtering=' + ($st -match '"filteringEnabled": true'))
L ('N2_owned=' + (@(Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.ServerAddresses -contains '127.0.0.1' }).Count))

L '=== N3 ==='
& $cli disable 2>&1 | Out-Null; Start-Sleep 1
$j3 = Start-Job { param($c) & $c enable 2>&1 | Out-String } -ArgumentList $cli
Start-Sleep -Milliseconds 50
$j4 = Start-Job { param($c) & $c disable 2>&1 | Out-String } -ArgumentList $cli
Wait-Job $j3,$j4 -Timeout 60 | Out-Null
L ('N3en=' + ((Receive-Job $j3) -replace "`r|`n",' '))
L ('N3dis=' + ((Receive-Job $j4) -replace "`r|`n",' '))
Remove-Job $j3,$j4 -Force
$st3 = & $cli status 2>&1 | Out-String
L ('N3_filtering=' + ($st3 -match '"filteringEnabled": true'))
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | ForEach-Object { L ('N3_dns=' + $_.InterfaceAlias + '|' + ($_.ServerAddresses -join ',')) }

L '=== N4 ==='
& $cli enable 2>&1 | Out-Null; Start-Sleep 1
$sj = 1..8 | ForEach-Object { Start-Job { param($c) & $c status 2>&1 | Out-Null; 'ok' } -ArgumentList $cli }
$rj = Start-Job { param($c) & $c reload-filters 2>&1 | Out-String } -ArgumentList $cli
Wait-Job ($sj + $rj) -Timeout 120 | Out-Null
L ('N4_reload=' + ((Receive-Job $rj) -replace "`r|`n",' '))
L ('N4_status_ok=' + (@($sj | Receive-Job | Where-Object { $_ -eq 'ok' }).Count))
Remove-Job ($sj + $rj) -Force
L ('N4_svc=' + (Get-Service BlockAdsService).Status)

L '=== N5 ==='
$j5 = Start-Job { param($c) & $c reload-filters 2>&1 | Out-String } -ArgumentList $cli
Start-Sleep -Milliseconds 30
$j6 = Start-Job { param($c) & $c disable 2>&1 | Out-String } -ArgumentList $cli
Wait-Job $j5,$j6 -Timeout 120 | Out-Null
L ('N5_reload=' + ((Receive-Job $j5) -replace "`r|`n",' '))
L ('N5_disable=' + ((Receive-Job $j6) -replace "`r|`n",' '))
Remove-Job $j5,$j6 -Force
L ('N5_svc=' + (Get-Service BlockAdsService).Status)
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | ForEach-Object { L ('N5_dns=' + $_.InterfaceAlias + '|' + ($_.ServerAddresses -join ',')) }

L '=== N6 ==='
try {
  Add-Type -AssemblyName System.Core
  $pc = New-Object System.IO.Pipes.NamedPipeClientStream('.', 'BlockAdsService', [System.IO.Pipes.PipeDirection]::InOut)
  $pc.Connect(3000)
  $b = [Text.Encoding]::UTF8.GetBytes("{`"version`":1,`"id`":`"d`",`"method`":`"status`"}`n")
  $pc.Write($b,0,$b.Length); $pc.Dispose()
  L 'N6_disconnect=ok'
} catch { L ('N6_err=' + $_.Exception.Message) }
L ('N6_svc=' + (Get-Service BlockAdsService).Status)
& $cli status 2>&1 | Select-Object -First 2 | ForEach-Object { L ("N6_status=$_") }

L '=== FILTER FAIL ==='
& $cli enable 2>&1 | Out-Null; Start-Sleep 2
$beforeF = & $cli status 2>&1 | Out-String
L ('filters_before_loaded=' + ($beforeF -match '"loaded": true'))
$rs = [runspacefactory]::CreateRunspace(); $rs.Open(); $ps = [powershell]::Create(); $ps.Runspace = $rs
[void]$ps.AddScript({
  $l = [System.Net.HttpListener]::new(); $l.Prefixes.Add('http://127.0.0.1:18765/'); $l.Start()
  for ($i=0; $i -lt 40; $i++) {
    try {
      $ctx = $l.GetContext(); $path = $ctx.Request.Url.AbsolutePath
      if ($path -eq '/cat.json') { $buf = [Text.Encoding]::UTF8.GetBytes('[{"id":"bad","name":"Bad","isEnabled":true,"category":"AD","bloomUrl":"http://127.0.0.1:18765/b.bloom","trieUrl":"http://127.0.0.1:18765/t.trie","ruleCount":1}]') }
      else { $buf = [Text.Encoding]::ASCII.GetBytes('BADMAGIC') }
      $ctx.Response.OutputStream.Write($buf,0,$buf.Length); $ctx.Response.Close()
    } catch { break }
  }
  try { $l.Stop() } catch {}
})
[void]$ps.BeginInvoke(); Start-Sleep 1
$raw = Get-Content $cfgPath -Raw | ConvertFrom-Json
$raw.filters.catalogUrl = 'http://127.0.0.1:18765/cat.json'
$raw.filters.enabledListIds = @('bad')
[System.IO.File]::WriteAllText($cfgPath, ($raw | ConvertTo-Json -Depth 6), (New-Object System.Text.UTF8Encoding $false))
$rf = & $cli reload-filters 2>&1 | Out-String
L ('reload_fail=' + ($rf -replace "`r|`n",' '))
$afterF = & $cli status 2>&1 | Out-String
L ('filters_after_loaded=' + ($afterF -match '"loaded": true'))
L ('filters_after_err=' + ($afterF -match 'lastUpdateError'))
$blk = Resolve-DnsName doubleclick.net -Type A -DnsOnly -EA SilentlyContinue | Select-Object -First 1
L ('block_still=' + $blk.IPAddress)
try { $ps.Stop(); $rs.Dispose() } catch {}

L '=== GATE L ==='
$raw = Get-Content $cfgPath -Raw | ConvertFrom-Json
$raw.filters.catalogUrl = $goodCatalog
$raw.filters.enabledListIds = @()
[System.IO.File]::WriteAllText($cfgPath, ($raw | ConvertTo-Json -Depth 6), (New-Object System.Text.UTF8Encoding $false))
& $cli enable 2>&1 | Out-Null; Start-Sleep 1
$st0 = & $cli status 2>&1 | Out-String
$cb0=0; $re0=0
if ($st0 -match 'netwatchCallbacks":\s*(\d+)') { $cb0=[int64]$Matches[1] }
if ($st0 -match 'netwatchReevaluations":\s*(\d+)') { $re0=[int64]$Matches[1] }
L ('L_cb0=' + $cb0 + ' L_re0=' + $re0)
$eth = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.InterfaceDescription -match 'I225' } | Select-Object -First 1
L ('L_eth=' + $eth.Name)
if ($eth) {
  Disable-NetAdapter -Name $eth.Name -Confirm:$false
  Start-Sleep 2
  Enable-NetAdapter -Name $eth.Name -Confirm:$false
  $deadline = (Get-Date).AddSeconds(45)
  do { Start-Sleep 1 } while (((Get-NetAdapter -Name $eth.Name).Status -ne 'Up') -and (Get-Date) -lt $deadline)
  Start-Sleep 3
  $st1 = & $cli status 2>&1 | Out-String
  $cb1=0; $re1=0
  if ($st1 -match 'netwatchCallbacks":\s*(\d+)') { $cb1=[int64]$Matches[1] }
  if ($st1 -match 'netwatchReevaluations":\s*(\d+)') { $re1=[int64]$Matches[1] }
  L ('L_cb1=' + $cb1 + ' L_re1=' + $re1)
  L ('L_cb_delta=' + ($cb1-$cb0) + ' L_re_delta=' + ($re1-$re0))
  Get-DnsClientServerAddress -InterfaceAlias $eth.Name -AddressFamily IPv4 -EA SilentlyContinue | ForEach-Object { L ('L_dns=' + (($_.ServerAddresses)-join ',')) }
  & $cli disable 2>&1 | ForEach-Object { L ("L_disable=$_") }
  Start-Sleep 1
  Get-DnsClientServerAddress -InterfaceAlias $eth.Name -AddressFamily IPv4 -EA SilentlyContinue | ForEach-Object { L ('L_dns_restored=' + (($_.ServerAddresses)-join ',')) }
}

& $cli disable 2>&1 | Out-Null
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | ForEach-Object { L ('final_dns=' + $_.InterfaceAlias + '|' + ($_.ServerAddresses -join ',')) }
L ('final_svc=' + (Get-Service BlockAdsService -EA SilentlyContinue).Status)
L '=== DONE ==='
