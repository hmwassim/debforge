package setup

func DefaultSteps() []Step {
	return []Step{
		&ReposStep{
			Sources: []RepoSource{
				{
					Path:    "/etc/apt/sources.list",
					Content: DebianSourcesList,
				},
			},
		},
		&I386Step{},
		&UpgradeStep{},
		&FirmwareStep{},
		&DevtoolsStep{},
		&KernelStep{},
	}
}

const DebianSourcesList = `deb http://deb.debian.org/debian trixie main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-updates main contrib non-free non-free-firmware
deb http://security.debian.org/debian-security/ trixie-security main contrib non-free non-free-firmware
deb http://deb.debian.org/debian trixie-backports main contrib non-free non-free-firmware
`
