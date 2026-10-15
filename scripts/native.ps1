# Dot-source from Windows workflow steps. ErrorActionPreference alone does
# not make native exit codes terminating errors on every PowerShell version.
function Invoke-CheckedNative {
    param(
        [Parameter(Mandatory = $true)][string]$Command,
        [string[]]$Arguments = @()
    )
    $ErrorActionPreference = 'Stop'
    $PSNativeCommandUseErrorActionPreference = $false
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "native command failed ($LASTEXITCODE): $Command"
    }
}
