package coresetup

const SourcesList = `deb http://deb.debian.org/debian trixie main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security/ trixie-security main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware
`

const ZramConfig = `[zram0]
zram-size = min(ram / 2, 8192)
compression-algorithm = zstd
swap-priority = 100
fs-type = swap
`

const ResolvedConfig = `[Resolve]
DNS=1.1.1.2#security.cloudflare-dns.com 1.0.0.2#security.cloudflare-dns.com 2606:4700:4700::1112#security.cloudflare-dns.com 2606:4700:4700::1002#security.cloudflare-dns.com
FallbackDNS=9.9.9.9#dns.quad9.net 149.112.112.112#dns.quad9.net 2620:fe::fe#dns.quad9.net
DNSOverTLS=yes
DNSSEC=yes
DNSStubListener=yes
MulticastDNS=no
Cache=yes
Domains=~.
`

const NmDNSConfig = `[main]
dns=systemd-resolved
`

const TimesyncdConfig = `[Time]
NTP=time.cloudflare.com
FallbackNTP=time.google.com 0.debian.pool.ntp.org 1.debian.pool.ntp.org 2.debian.pool.ntp.org 3.debian.pool.ntp.org
`

const AudioLimitsConfig = `@audio - rtprio 99
`

const FontConfig = `<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>

  <selectfont>
    <rejectfont>
      <glob>*NotoNastaliq*</glob>
    </rejectfont>
  </selectfont>

  <alias>
    <family>sans-serif</family>
    <prefer>
      <family>Arimo</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>serif</family>
    <prefer>
      <family>Tinos</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>Sans</family>
    <prefer>
      <family>Arimo</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <alias>
    <family>monospace</family>
    <prefer>
      <family>Cousine</family>
      <family>Noto Sans Arabic</family>
    </prefer>
  </alias>

  <match>
    <test name="family"><string>Arial</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Helvetica</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Verdana</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Tahoma</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Comic Sans MS</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Arimo</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times New Roman</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Times</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Tinos</string>
    </edit>
  </match>
  <match>
    <test name="family"><string>Courier New</string></test>
    <edit name="family" mode="assign" binding="strong">
      <string>Cousine</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>ar</string></test>
    <test name="spacing" compare="eq"><int>100</int></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Sans Arabic UI</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="family"><string>emoji</string></test>
    <edit name="family" mode="prepend" binding="strong">
      <string>Noto Color Emoji</string>
    </edit>
  </match>

  <match target="pattern">
    <test name="lang" compare="contains"><string>zh</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK SC</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ja</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK JP</string>
    </edit>
  </match>
  <match target="pattern">
    <test name="lang" compare="contains"><string>ko</string></test>
    <edit name="family" mode="append" binding="weak">
      <string>Noto Sans CJK KR</string>
    </edit>
  </match>

</fontconfig>
`
