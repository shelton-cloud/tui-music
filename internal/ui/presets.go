package ui

import (
	"fmt"
	"math/rand"
	"strings"

	"charm.land/lipgloss/v2"

	"tui-music/internal/library"
	"tui-music/internal/youtube"
)

// Preset defines one static hardware-style station button (keys 1-9, 0) on the command strip.
// Each preset filters the local NAS library using static artist/album/path keywords and
// falls back to a curated YouTube Music search query when the NAS is offline.
type Preset struct {
	Key        string
	Label      string
	ShortLabel string
	FullName   string
	Group      string // "genre" (1-5) or "decade" (6-0)
	YTQuery    string
	Keywords   []string
}

const maxPresetQueueTracks = 50

// DefaultPresets defines the 10 static command-strip presets arranged in keyboard order (1-9, 0):
//   - Keys 1-5: Genre / Style bank (Rock, Metal, Indie, Beats, Lounge)
//   - Keys 6-0: Decade bank (60s, 70s, 80s, 90s, 00s) matching the decade digit directly
var DefaultPresets = []Preset{
	{
		Key:        "1",
		Label:      "ROCK",
		ShortLabel: "RCK",
		FullName:   "Alt & Modern Rock",
		Group:      "genre",
		YTQuery:    "alternative modern rock greatest hits",
		Keywords: []string{
			"foo fighters", "incubus", "coheed and cambria", "thrice", "dredg",
			"muse", "radiohead", "the mars volta", "circa survive", "the dear hunter",
			"sparta", "steve vai", "ac/dc", "ac_dc", "a perfect circle",
			"rage against the machine", "nine inch nails", "red hot chili peppers",
			"tenacious d", "the killers", "weezer",
		},
	},
	{
		Key:        "2",
		Label:      "METAL",
		ShortLabel: "MTL",
		FullName:   "Metal & Heavy",
		Group:      "genre",
		YTQuery:    "heavy metal hard rock playlist",
		Keywords: []string{
			"metallica", "pantera", "tool", "deftones", "sevendust", "korn",
			"limp bizkit", "killswitch engage", "atreyu", "underoath",
			"system of a down", "mudvayne", "linkin park", "slipknot",
			"august burns red", "every time i die", "periphery", "dream theater",
			"protest the hero", "dethklok", "p.o.d.", "staind", "poison the well",
			"marilyn manson", "austrian death machine", "glassjaw", "cave in",
		},
	},
	{
		Key:        "3",
		Label:      "INDIE",
		ShortLabel: "IND",
		FullName:   "Indie, Jam & Chill",
		Group:      "genre",
		YTQuery:    "indie acoustic jam band chill mix",
		Keywords: []string{
			"dave matthews", "john mayer", "death cab for cutie", "minus the bear",
			"coldplay", "copeland", "mutemath", "the fray", "the derek trucks band",
			"ben folds five", "jack's mannequin", "margot & the nuclear",
			"counting crows", "hootie & the blowfish", "the rocket summer",
			"anthony green", "bryan shelton",
		},
	},
	{
		Key:        "4",
		Label:      "BEATS",
		ShortLabel: "BTS",
		FullName:   "Hip-Hop & Electronic",
		Group:      "genre",
		YTQuery:    "classic hip hop electronic groove mix",
		Keywords: []string{
			"daft punk", "kanye west", "outkast", "prodigy", "snoop dogg",
			"the notorious b.i.g.", "lupe fiasco", "head automatica",
			"the secret handshake", "cobra starship",
		},
	},
	{
		Key:        "5",
		Label:      "LOUNGE",
		ShortLabel: "LNG",
		FullName:   "Jazz, Blues & Crooners",
		Group:      "genre",
		YTQuery:    "classic crooners jazz blues lounge mix",
		Keywords: []string{
			"frank sinatra", "dean martin", "michael buble", "b.b. king",
			"chuck berry", "the derek trucks band", "santana", "jazz", "blues",
			"countdown singers",
		},
	},
	{
		Key:        "6",
		Label:      "60s",
		ShortLabel: "60s",
		FullName:   "60s Classics",
		Group:      "decade",
		YTQuery:    "60s classic rock oldies hits",
		Keywords: []string{
			"beatles", "jimi hendrix", "the rolling stones", "chuck berry",
			"frank sinatra", "dean martin", "b.b. king", "60s", "60's",
		},
	},
	{
		Key:        "7",
		Label:      "70s",
		ShortLabel: "70s",
		FullName:   "70s Classic Rock",
		Group:      "decade",
		YTQuery:    "70s classic rock greatest hits",
		Keywords: []string{
			"led zeppelin", "ac/dc", "ac_dc", "aerosmith", "boston", "queen",
			"pink floyd", "santana", "the rolling stones", "eagles",
			"fleetwood mac", "lynyrd skynyrd", "70s", "70's",
		},
	},
	{
		Key:        "8",
		Label:      "80s",
		ShortLabel: "80s",
		FullName:   "80s Hits & Rock",
		Group:      "decade",
		YTQuery:    "80s rock pop greatest hits",
		Keywords: []string{
			"van halen", "michael jackson", "prince", "the police", "u2",
			"huey lewis", "genesis", "metallica", "weird al", "steve vai",
			"bon jovi", "journey", "def leppard", "guns n' roses", "80s", "80's",
		},
	},
	{
		Key:        "9",
		Label:      "90s",
		ShortLabel: "90s",
		FullName:   "90s Alt & Rock",
		Group:      "decade",
		YTQuery:    "90s alternative rock greatest hits",
		Keywords: []string{
			"90's music", "90s", "foo fighters", "green day", "hootie & the blowfish",
			"counting crows", "third eye blind", "weezer", "sublime",
			"red hot chili peppers", "smashing pumpkins", "nine inch nails",
			"rage against the machine", "offspring", "dave matthews band",
			"ben folds five", "nirvana", "pearl jam", "soundgarden",
		},
	},
	{
		Key:        "0",
		Label:      "00s",
		ShortLabel: "00s",
		FullName:   "00s Emo & Pop-Punk",
		Group:      "decade",
		YTQuery:    "2000s pop punk emo alternative rock hits",
		Keywords: []string{
			"blink-182", "fall out boy", "jimmy eat world", "dashboard confessional",
			"paramore", "taking back sunday", "the killers", "all time low",
			"yellowcard", "new found glory", "motion city soundtrack",
			"the starting line", "anberlin", "the used", "hawthorne heights",
			"senses fail", "cartel", "the academy is", "the all-american rejects",
			"bayside", "story of the year", "say anything", "emery", "finch",
			"saosin", "armor for sleep", "further seems forever", "thursday",
			"the maine", "four year strong", "set your goals", "angels & airwaves",
			"00s", "2000s",
		},
	},
}

// FindPreset looks up a preset by its single-digit key ("0"-"9").
func FindPreset(key string) (Preset, bool) {
	for _, p := range DefaultPresets {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// MatchPresetSongs filters songs matching any static keyword in preset (case-insensitive)
// across Artist, Album, and Path, shuffles them, and caps the result to limit tracks.
func MatchPresetSongs(songs []library.Song, preset Preset, rng *rand.Rand, limit int) []library.Song {
	if len(songs) == 0 || len(preset.Keywords) == 0 {
		return nil
	}
	var matched []library.Song
	for _, s := range songs {
		haystack := strings.ToLower(s.Artist + " " + s.Album + " " + s.Path)
		for _, kw := range preset.Keywords {
			if strings.Contains(haystack, kw) {
				matched = append(matched, s)
				break
			}
		}
	}
	if len(matched) == 0 {
		return nil
	}
	if rng != nil && len(matched) > 1 {
		rng.Shuffle(len(matched), func(i, j int) {
			matched[i], matched[j] = matched[j], matched[i]
		})
	}
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	return matched
}

// renderPresetStrip renders the 0-9 hardware preset command strip.
// When height >= 3 and width >= 80, it renders tactile 3-row CRT push-buttons with
// glowing active LEDs and distinct Genre (1-5) vs. Decade (6-0) color accents.
// On compact terminals it gracefully falls back to a 1-row pill bar so all 10 presets stay visible.
func (m *Model) renderPresetStrip(totalWidth, stripHeight int) string {
	if stripHeight < 3 || totalWidth < 80 {
		return m.renderPresetStripCompact(totalWidth)
	}
	return m.renderPresetStripButtons(totalWidth)
}

func (m *Model) renderPresetStripButtons(totalWidth int) string {
	count := len(DefaultPresets)
	// Distribute exact totalWidth across all 10 buttons with 0 overflow.
	baseW := totalWidth / count
	rem := totalWidth % count

	buttons := make([]string, 0, count)
	for i, p := range DefaultPresets {
		btnW := baseW
		if i < rem {
			btnW++
		}
		isActive := m.activePreset == p.Key
		isLoading := m.presetLoading == p.Key

		// Bank color theme: Genres (1-5) use Cyan/Purple; Decades (6-0) use Amber/Pink.
		borderColor := colorPurple
		keyBg := colorPurple
		labelFg := colorCyan
		if p.Group == "decade" {
			borderColor = colorPink
			keyBg = colorAmber
			labelFg = colorAmber
		}

		led := "─"
		ledColor := borderColor
		if isLoading {
			borderColor = colorAmber
			keyBg = colorAmber
			labelFg = colorAmber
			led = "◌"
			ledColor = colorAmber
		} else if isActive {
			borderColor = colorPhosphor
			keyBg = colorPhosphor
			labelFg = colorPhosphor
			led = "●"
			ledColor = colorPhosphor
		}

		innerW := max(4, btnW-2) // inside left & right box borders

		// Top border: ╭●[7]──╮  or ╭─[7]──╮
		keyBadge := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDarkBg).
			Background(keyBg).
			Render("[" + p.Key + "]")

		borderSt := lipgloss.NewStyle().Foreground(borderColor)
		ledSt := lipgloss.NewStyle().Bold(true).Foreground(ledColor)

		// Inner top occupies: led (1) + "[X]" (3) + trailing dashes
		dashCount := innerW - 4
		if dashCount < 0 {
			dashCount = 0
		}
		topLine := borderSt.Render("╭") + ledSt.Render(led) + keyBadge + borderSt.Render(strings.Repeat("─", dashCount)+"╮")

		// Middle label row: centered inside box, highlighted background when active
		lbl := p.Label
		if lipgloss.Width(lbl) > innerW {
			lbl = p.ShortLabel
		}
		lbl = truncateStr(lbl, innerW)
		padTotal := innerW - lipgloss.Width(lbl)
		padL := padTotal / 2
		padR := padTotal - padL

		midText := strings.Repeat(" ", padL) + lbl + strings.Repeat(" ", padR)
		midSt := lipgloss.NewStyle().Bold(isActive || isLoading).Foreground(labelFg)
		if isActive {
			midSt = midSt.Background(colorHeaderBg)
		}
		midLine := borderSt.Render("│") + midSt.Render(midText) + borderSt.Render("│")

		// Bottom border: ╰──────╯
		botLine := borderSt.Render("╰" + strings.Repeat("─", innerW) + "╯")

		btn := strings.Join([]string{topLine, midLine, botLine}, "\n")
		buttons = append(buttons, btn)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, buttons...)
	return lipgloss.NewStyle().MaxWidth(totalWidth).MaxHeight(3).Render(row)
}

func (m *Model) renderPresetStripCompact(totalWidth int) string {
	sepStyle := lipgloss.NewStyle().Background(colorDarkBg)
	sep := sepStyle.Render(" ")

	var parts []string
	used := 1 // leading space
	for _, p := range DefaultPresets {
		isActive := m.activePreset == p.Key
		isLoading := m.presetLoading == p.Key

		keyBg := colorPurple
		lblFg := colorCyan
		if p.Group == "decade" {
			keyBg = colorAmber
			lblFg = colorAmber
		}
		if isLoading {
			keyBg = colorAmber
			lblFg = colorAmber
		} else if isActive {
			keyBg = colorPhosphor
			lblFg = colorPhosphor
		}

		keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorDarkBg).Background(keyBg)
		lblStyle := lipgloss.NewStyle().Bold(isActive).Foreground(lblFg).Background(colorHeaderBg)

		lbl := p.ShortLabel
		if totalWidth >= 90 {
			lbl = p.Label
		}
		keyTxt := p.Key
		if isActive {
			keyTxt = "●" + p.Key
		}
		btn := keyStyle.Render(keyTxt) + lblStyle.Render(lbl)
		w := lipgloss.Width(keyTxt) + lipgloss.Width(lbl)
		if used+w+1 > totalWidth {
			break
		}
		parts = append(parts, btn)
		used += w + 1
	}

	row := sep + strings.Join(parts, sep)
	if w := lipgloss.Width(row); w < totalWidth {
		row += sepStyle.Render(strings.Repeat(" ", totalWidth-w))
	}
	return lipgloss.NewStyle().MaxWidth(totalWidth).MaxHeight(1).Render(row)
}

// presetResultsMsg carries async YouTube Music search results for a preset when NAS is offline.
type presetResultsMsg struct {
	preset  Preset
	results []youtube.Result
	err     error
}

func formatPresetBreadcrumb(p Preset, count int, isLocal bool) string {
	src := "NAS"
	if !isLocal {
		src = "YT MUSIC"
	}
	return fmt.Sprintf("󰎆 PRESET [%s] %s • %d %s TRACKS", p.Key, strings.ToUpper(p.FullName), count, src)
}
