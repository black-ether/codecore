using namespace System.Collections.Generic
Import-Module MyModule

$ScriptVar = 42
$_private = "hidden"
$sb = { param($x) $x * 2 }

$TRAP_STRING = "function Fake() {}"
# function commented() {}

function Get-Thing {
    param(
        [string]$Name,
        [int]$Count = 1
    )
    $local = $Name
    return $local
}

function Invoke-Helper {
    [CmdletBinding()]
    param()
    Write-Host "hi"
}

filter Get-Even {
    param([int]$n)
    if ($n % 2 -eq 0) { $n }
}

class Widget {
    [string]$Name
    [int]$Size = 10

    Widget([string]$name) {
        $this.Name = $name
    }

    [string] GetLabel() {
        return $this.Name
    }

    hidden [void] Reset() {
        $this.Name = ""
    }
}
