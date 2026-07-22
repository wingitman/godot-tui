$dir = Join-Path $env:LOCALAPPDATA 'Programs\godot-tui'
Remove-Item (Join-Path $dir 'godot-tui.exe') -Force -ErrorAction SilentlyContinue
Write-Host "Removed $dir\godot-tui.exe"
