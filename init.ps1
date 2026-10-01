# enved's PowerShell wrapper, printed by `enved init pwsh`.
# enved.exe changes the saved variables, but a child process cannot touch this shell's
# environment. This compares the saved variables before and after and applies the
# difference here: a changed variable gets its new value, a removed one goes. Path is
# merged instead, so entries only this session has (mise's, a venv's, ...) stay: added
# entries go to the end, removed ones are dropped.
function enved {
  # The saved variables as a new process gets them: Machine, then User over it, expanded.
  $saved = {
    $vars = @{}
    foreach ($scope in 'Machine', 'User') {
      $all = [Environment]::GetEnvironmentVariables($scope)
      foreach ($name in $all.Keys) { $vars[$name] = $all[$name] }
    }
    $vars['Path'] = @('Machine', 'User' |
      ForEach-Object { [Environment]::GetEnvironmentVariable('Path', $_) -split ';' } |
      Where-Object { $_ })
    $vars
  }

  $before = & $saved
  enved.exe @args
  $after = & $saved

  foreach ($name in @($before.Keys) + @($after.Keys) | Sort-Object -Unique) {
    if ($name -eq 'Path') { continue }
    if (-not $after.ContainsKey($name)) {
      [Environment]::SetEnvironmentVariable($name, $null)
    } elseif (-not $before.ContainsKey($name) -or $before[$name] -cne $after[$name]) {
      [Environment]::SetEnvironmentVariable($name, $after[$name])
    }
  }

  $removed = @($before['Path'] | Where-Object { $_ -notin $after['Path'] })
  $added = @($after['Path'] | Where-Object { $_ -notin $before['Path'] })
  $kept = @($env:PATH -split ';' | Where-Object { $_ -and $_ -notin $removed })
  $env:PATH = ($kept + @($added | Where-Object { $_ -notin $kept })) -join ';'
}
