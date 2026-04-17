// SPDX-License-Identifier: GPL-3.0-or-later
package protocol

import "math"

const earthRadiusMeters = 6371000.0

// HaversineMeters returns the great-circle distance in meters between two points.
func HaversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := deg2rad(lat2 - lat1)
	dLon := deg2rad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(deg2rad(lat1))*math.Cos(deg2rad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMeters * c
}

func deg2rad(d float64) float64 {
	return d * math.Pi / 180
}

// HaversineKm returns distance in kilometers.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	return HaversineMeters(lat1, lon1, lat2, lon2) / 1000.0
}

// InsideBoundingBox checks if a point is inside the mission bounding box.
func InsideBoundingBox(lat, lon, north, south, east, west float64) bool {
	return lat <= north && lat >= south && lon <= east && lon >= west
}
