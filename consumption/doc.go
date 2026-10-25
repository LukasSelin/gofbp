// Package consumption is the FBP System's fuel consumption and fire intensity:
// surface fuel consumption SFC, crown fuel consumption CFC, total fuel
// consumption TFC (consumption.go), and head fire intensity HFI
// (intensity.go).
//
// They are functions of CFB rather than part of the crown-fire threshold,
// because they are a separate quantity from spread. SFC is also an INPUT to the
// threshold — Crown.SFC in package crown — but crown does not call
// SurfaceFuelConsumption: the grass fuel load it needs is the caller's.
package consumption
