SELECT mss."solarSystemID" FROM
"mapRegions" mr JOIN "mapSolarSystems" mss ON mss."regionID" = mr."regionID"
WHERE mr."regionName" = 'Fountain';
