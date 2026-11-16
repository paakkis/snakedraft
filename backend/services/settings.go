package services

// ScoreSettings says what a skill score is built from. The zero value scores
// nobody: start from DefaultScoreSettings.
type ScoreSettings struct {
	// Which ranks count. With both, the better one is used.
	UseFaceit  bool `json:"use_faceit"`
	UsePremier bool `json:"use_premier"`
	// UseSteam lets Steam lifetime stats in: as a small share next to a
	// better source, and as the whole score for players with nothing else.
	UseSteam bool `json:"use_steam"`

	// Weights of the components. Only their ratios matter. A component with
	// weight 0 is left out.
	WeightRank        float64 `json:"weight_rank"`
	WeightAim         float64 `json:"weight_aim"`
	WeightPositioning float64 `json:"weight_positioning"`
	WeightUtility     float64 `json:"weight_utility"`
	WeightOpening     float64 `json:"weight_opening"`
	WeightForm        float64 `json:"weight_form"`

	// ScaleByConfidence makes a rank count for less when few matches are
	// behind it or it has not been played for in a long time.
	ScaleByConfidence bool `json:"scale_by_confidence"`
	// AdjustForPerformance moves a Faceit ELO towards the one the player
	// performed like against their recent lobbies.
	AdjustForPerformance bool `json:"adjust_for_performance"`
	// AdjustForMechanics raises a rank whose player aims and positions
	// clearly better than is typical for it.
	AdjustForMechanics bool `json:"adjust_for_mechanics"`
}

// DefaultScoreSettings is the full model: what the player list shows.
func DefaultScoreSettings() ScoreSettings {
	return ScoreSettings{
		UseFaceit:  true,
		UsePremier: true,
		UseSteam:   true,

		WeightRank:        0.55,
		WeightAim:         0.15,
		WeightPositioning: 0.10,
		WeightUtility:     0.04,
		WeightOpening:     0.08,
		WeightForm:        0.08,

		ScaleByConfidence:    true,
		AdjustForPerformance: true,
		AdjustForMechanics:   true,
	}
}

// maxWeight caps a component weight. Only ratios matter, so this is just a
// guard against nonsense input.
const maxWeight = 100.0

// Sanitized returns the settings with every weight forced into 0–maxWeight.
func (s ScoreSettings) Sanitized() ScoreSettings {
	for _, w := range []*float64{
		&s.WeightRank, &s.WeightAim, &s.WeightPositioning,
		&s.WeightUtility, &s.WeightOpening, &s.WeightForm,
	} {
		// The negated comparison also catches NaN.
		if !(*w > 0) {
			*w = 0
		} else if *w > maxWeight {
			*w = maxWeight
		}
	}
	return s
}

// DraftSettings is everything that decides how a draft turns out.
type DraftSettings struct {
	ScoreSettings

	// MaxSpread is the largest gap between the strongest and weakest team
	// average that still counts as fair when honouring teammate requests.
	MaxSpread float64 `json:"max_spread"`
	// HonorRequests keeps requested teammates together where that is fair.
	HonorRequests bool `json:"honor_requests"`
	// FallbackToFullModel scores a player these settings cannot rate (say,
	// a Faceit-only draft and a player who is not on Faceit) with the full
	// model instead of leaving them out of the draft.
	FallbackToFullModel bool `json:"fallback_to_full_model"`
}

func DefaultDraftSettings() DraftSettings {
	return DraftSettings{
		ScoreSettings:       DefaultScoreSettings(),
		MaxSpread:           DefaultMaxSpread,
		HonorRequests:       true,
		FallbackToFullModel: true,
	}
}

// maxMaxSpread is the whole score scale: a larger limit means nothing.
const maxMaxSpread = 100.0

// Sanitized returns the settings with every number forced into its range.
func (s DraftSettings) Sanitized() DraftSettings {
	s.ScoreSettings = s.ScoreSettings.Sanitized()
	if !(s.MaxSpread > 0) {
		s.MaxSpread = DefaultMaxSpread
	} else if s.MaxSpread > maxMaxSpread {
		s.MaxSpread = maxMaxSpread
	}
	return s
}
