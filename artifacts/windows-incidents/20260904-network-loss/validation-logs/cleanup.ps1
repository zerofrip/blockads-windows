$ErrorActionPreference='Continue'
$cli='C:\Users\zerof\blockads-windows-val\artifacts-tmp\blockads-cli.exe'
$exe='C:\Users\zerof\blockads-windows-val\artifacts-tmp\blockads-service.exe'
$log='C:\Users\zerof\blockads-windows-val\artifacts-tmp\cleanup.txt'
Set-Content -Encoding utf8 $log 'cleanup2'
try { & $cli disable 2>&1 | ForEach-Object { Add-Content $log "dis=$_" } } catch {}
Start-Sleep 1
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object { $_.ServerAddresses -contains '127.0.0.1' } | ForEach-Object {
  Add-Content $log ('reset ' + $_.InterfaceAlias)
  Set-DnsClientServerAddress -InterfaceAlias $_.InterfaceAlias -ResetServerAddresses
}
try { & $exe stop 2>&1 | Out-Null } catch {}
Start-Sleep 1
Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object ServerAddresses | ForEach-Object {
  Add-Content $log ($_.InterfaceAlias + '=' + ($_.ServerAddresses -join ','))
}
Add-Content $log 'done'
