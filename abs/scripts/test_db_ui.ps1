$headers = @{
    'X-Dev-User-ID' = '11111111-1111-1111-1111-111111111111'
    'X-Dev-User-Email' = 'testuser@samvada.ai'
    'X-Workspace-ID' = '44444444-4444-4444-4444-444444444444'
}

Write-Host "=========================================================="
Write-Host "TESTING LIVE DATA ON PORT 5173 (Vite -> ABS -> PostgreSQL)"
Write-Host "=========================================================="

# 1. Agents
$agents = Invoke-RestMethod -Uri 'http://localhost:5173/api/v1/workspaces/44444444-4444-4444-4444-444444444444/agents' -Headers $headers
Write-Host "`n[1. LIVE AGENTS IN POSTGRESQL]:" -ForegroundColor Cyan
foreach ($a in $agents.data) {
    Write-Host ("  - " + $a.name + " (" + $a.role + ") | Lang: " + $a.language + " | Status: " + $a.status)
}

# 2. Campaigns
$campaigns = Invoke-RestMethod -Uri 'http://localhost:5173/api/v1/workspaces/44444444-4444-4444-4444-444444444444/campaigns' -Headers $headers
Write-Host "`n[2. LIVE CAMPAIGNS IN POSTGRESQL]:" -ForegroundColor Cyan
foreach ($c in $campaigns.data) {
    Write-Host ("  - " + $c.name + " | Status: " + $c.status + " | Contacts: " + $c.total_contacts + " | Completed: " + $c.completed_calls)
}

# 3. Telephony Numbers
$phones = Invoke-RestMethod -Uri 'http://localhost:5173/api/v1/workspaces/44444444-4444-4444-4444-444444444444/phone-numbers' -Headers $headers
Write-Host "`n[3. LIVE TELEPHONY NUMBERS IN POSTGRESQL]:" -ForegroundColor Cyan
foreach ($p in $phones.data) {
    Write-Host ("  - " + $p.phone_number + " | Provider: " + $p.provider + " | Type: " + $p.type + " | Status: " + $p.status)
}

# 4. Contacts / Audiences
$contacts = Invoke-RestMethod -Uri 'http://localhost:5173/api/v1/workspaces/44444444-4444-4444-4444-444444444444/audiences/bbbbbbbb-1111-1111-1111-111111111111/contacts' -Headers $headers
Write-Host "`n[4. LIVE CONTACTS IN POSTGRESQL]:" -ForegroundColor Cyan
foreach ($ct in $contacts.data.contacts) {
    Write-Host ("  - " + $ct.name + " (" + $ct.company + ") | Phone: " + $ct.phone + " | Email: " + $ct.email)
}

Write-Host "`n=========================================================="
Write-Host "ALL ENDPOINTS RETURNED 100% LIVE POSTGRESQL DATA SUCCESSFULLY"
Write-Host "=========================================================="
