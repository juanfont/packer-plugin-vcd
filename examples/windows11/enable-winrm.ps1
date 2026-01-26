# Enable WinRM for Packer provisioning
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Force
winrm quickconfig -q
winrm set winrm/config/service '@{AllowUnencrypted="true"}'
winrm set winrm/config/service/auth '@{Basic="true"}'
Set-Service -Name WinRM -StartupType Automatic
Restart-Service WinRM
netsh advfirewall firewall add rule name="WinRM" dir=in action=allow protocol=TCP localport=5985
