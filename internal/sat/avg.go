package sat

type EMA struct {
	decay float64
	value float64
	init  bool
}

func NewEMA(decay float64) EMA {
	return EMA{decay: decay}
}

func (ema *EMA) Add(x float64) {
	if !ema.init {
		ema.init = true
		ema.value = x
	} else {
		ema.value = ema.decay*ema.value + x*(1-ema.decay)
	}
}

func (ema *EMA) Val() float64 {
	return ema.value
}
