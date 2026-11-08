"""Reference values for geo/ tests: shapely 2.x, pyproj (WGS84 Geod), scikit-learn.

Run: refenv/bin/python -I gen_refs.py <out_dir>
"""
import json
import math
import random
import sys

import numpy as np
import shapely
from shapely.geometry import Point, LineString, Polygon, MultiPolygon, MultiPoint, mapping
from pyproj import Geod
from sklearn.cluster import KMeans, DBSCAN
from sklearn.metrics.pairwise import haversine_distances

out = sys.argv[1]
rng = random.Random(7)


def star(cx, cy, r, n):
    pts = []
    for i in range(n):
        a = 2 * math.pi * i / n
        rr = r * rng.uniform(0.4, 1.0)
        pts.append((round(cx + rr * math.cos(a), 6), round(cy + rr * math.sin(a), 6)))
    return pts


def rand_poly():
    cx, cy = rng.uniform(-5, 5), rng.uniform(-5, 5)
    shell = star(cx, cy, rng.uniform(2, 6), rng.randint(5, 14))
    holes = []
    if rng.random() < 0.4:
        h = star(cx, cy, 0.8, rng.randint(3, 7))
        holes.append(h)
    p = Polygon(shell, holes)
    if not p.is_valid:
        p = Polygon(shell)
    if not p.is_valid:
        return rand_poly()
    return p


def rand_geom():
    k = rng.random()
    if k < 0.4:
        return Point(round(rng.uniform(-10, 10), 6), round(rng.uniform(-10, 10), 6))
    if k < 0.7:
        return LineString([(round(rng.uniform(-10, 10), 6), round(rng.uniform(-10, 10), 6)) for _ in range(rng.randint(2, 4))])
    cx, cy = rng.uniform(-6, 6), rng.uniform(-6, 6)
    p = Polygon(star(cx, cy, rng.uniform(0.5, 3), rng.randint(3, 8)))
    return p if p.is_valid else Point(cx, cy)


cases = []
for _ in range(400):
    a = rand_poly()
    if rng.random() < 0.2:
        b2 = rand_poly()
        if not a.intersects(b2):
            a = MultiPolygon([a, b2])
    b = rand_geom()
    cases.append({"a": mapping(a), "b": mapping(b), "covers": bool(a.covers(b)), "intersects": bool(a.intersects(b))})
# geometries near a point inside a: about half are covered
for _ in range(250):
    a = rand_poly()
    p = a.representative_point()
    s = rng.uniform(0.1, 2.5)
    k = rng.random()
    if k < 0.3:
        b = Point(round(p.x + rng.uniform(-s, s), 6), round(p.y + rng.uniform(-s, s), 6))
    elif k < 0.6:
        b = LineString([(round(p.x + rng.uniform(-s, s), 6), round(p.y + rng.uniform(-s, s), 6)) for _ in range(3)])
    else:
        b = Polygon(star(p.x, p.y, s, rng.randint(3, 7)))
        if not b.is_valid:
            continue
    cases.append({"a": mapping(a), "b": mapping(b), "covers": bool(a.covers(b)), "intersects": bool(a.intersects(b))})
# boundary cases: points on edges and vertices, a line along an edge, a polygon equal to a hole
sq = Polygon([(0, 0), (4, 0), (4, 4), (0, 4)], [[(1, 1), (2, 1), (2, 2), (1, 2)]])
for b in [Point(0, 2), Point(4, 4), Point(1.5, 1), Point(1.5, 1.5), LineString([(0, 0), (4, 0)]),
          LineString([(0, 0), (4, 4)]), Polygon([(1, 1), (2, 1), (2, 2), (1, 2)]),
          Polygon([(0.5, 0.5), (3, 0.5), (3, 3), (0.5, 3)]), Polygon([(2, 2), (3, 2), (3, 3), (2, 3)]),
          LineString([(4, 0), (5, 0)]), Point(5, 5)]:
    cases.append({"a": mapping(sq), "b": mapping(b), "covers": bool(sq.covers(b)), "intersects": bool(sq.intersects(b))})
# concave: a chord between two boundary points that leaves the polygon
cv = Polygon([(0, 0), (4, 0), (4, 4), (2, 1), (0, 4)])
for b in [LineString([(0, 4), (4, 4)]), LineString([(0, 0), (4, 0)]), LineString([(1, 0.5), (3, 0.5)])]:
    cases.append({"a": mapping(cv), "b": mapping(b), "covers": bool(cv.covers(b)), "intersects": bool(cv.intersects(b))})

# centroid and hull
cent = []
for _ in range(60):
    g = rand_poly() if rng.random() < 0.6 else rand_geom()
    c = g.centroid
    cent.append({"g": mapping(g), "centroid": [c.x, c.y], "area": g.area})
hulls = []
for _ in range(40):
    pts = [(round(rng.uniform(-10, 10), 4), round(rng.uniform(-10, 10), 4)) for _ in range(rng.randint(3, 40))]
    h = MultiPoint(pts).convex_hull
    hulls.append({"points": pts, "hull": mapping(h), "area": h.area})

# geodesic area (pyproj, WGS84, geodesic edges)
geod = Geod(ellps="WGS84")
areas = []
for name, ring in [
    ("1 degree square at the equator", [(0, 0), (1, 0), (1, 1), (0, 1), (0, 0)]),
    ("Colorado (corners)", [(-109.05, 36.99), (-102.04, 36.99), (-102.04, 41.0), (-109.05, 41.0), (-109.05, 36.99)]),
    ("city block, Berlin", [(13.400, 52.520), (13.402, 52.520), (13.402, 52.521), (13.400, 52.521), (13.400, 52.520)]),
    ("triangle 0,0 10,0 0,10", [(0, 0), (10, 0), (0, 10), (0, 0)]),
    ("high latitude", [(10, 70), (30, 70), (30, 80), (10, 80), (10, 70)]),
    ("southern, clockwise", [(150, -35), (150, -30), (155, -30), (155, -35), (150, -35)]),
    ("near antimeridian", [(178, -10), (179.9, -10), (179.9, -8), (178, -8), (178, -10)]),
    ("large: 60 degrees wide", [(-30, -20), (30, -20), (30, 20), (-30, 20), (-30, -20)]),
]:
    lons = [p[0] for p in ring]
    lats = [p[1] for p in ring]
    a, perim = geod.polygon_area_perimeter(lons, lats)
    areas.append({"name": name, "ring": ring, "area": abs(a), "perimeter": perim})
holed = Polygon([(0, 0), (1, 0), (1, 1), (0, 1)], [[(0.25, 0.25), (0.75, 0.25), (0.75, 0.75), (0.25, 0.75)]])
a, _ = geod.geometry_area_perimeter(shapely.geometry.polygon.orient(holed))  # pyproj needs holes clockwise
areas.append({"name": "square with a hole", "geom": mapping(holed), "area": abs(a)})

# haversine distances (sklearn), on R = 6371008.8
R = 6371008.8
pairs = [
    ((50.0663889, -5.7147222), (58.6438889, -3.07)),  # Land's End to John o' Groats
    ((0, 179.5), (0, -179.5)),  # across the antimeridian
    ((90, 0), (-90, 0)),  # pole to pole
    ((89.9, 0), (89.9, 180)),  # over the north pole
    ((-89.99, 45), (-89.99, -135)),  # over the south pole
    ((52.52, 13.405), (48.1351, 11.582)),  # Berlin to Munich
    ((40.7128, -74.006), (-33.8688, 151.2093)),  # New York to Sydney
]
dists = []
for a, b in pairs:
    d = haversine_distances(np.radians([a, b]))[0][1] * R
    dists.append({"a": {"lat": a[0], "lon": a[1]}, "b": {"lat": b[0], "lon": b[1]}, "dist": d})

# k-means: sklearn Lloyd from fixed initial centers, tol 0 (strict convergence)
krng = np.random.default_rng(3)
blobs = np.vstack([krng.normal(loc, 1.2, size=(60, 2)) for loc in [(0, 0), (8, 1), (3, 9)]]).round(4)
init_idx = [0, 1, 2]  # three points of the first blob: a poor start Lloyd must recover from
km = KMeans(n_clusters=3, init=blobs[init_idx], n_init=1, algorithm="lloyd", tol=0, max_iter=300).fit(blobs)
kmeans = {"points": blobs.tolist(), "init": init_idx, "labels": km.labels_.tolist(),
          "centers": km.cluster_centers_.tolist(), "inertia": float(km.inertia_)}

# DBSCAN: haversine on lat/lon (eps in meters / R) and euclidean on x/y
drng = np.random.default_rng(5)
centers = [(52.52, 13.40), (52.50, 13.45), (48.14, 11.58), (0.0, 179.99)]
ll = []
for c in centers:
    for _ in range(25):
        ll.append((round(c[0] + drng.normal(0, 0.004), 6), round(((c[1] + drng.normal(0, 0.006) + 180) % 360) - 180, 6)))
for _ in range(15):
    ll.append((round(drng.uniform(-60, 60), 6), round(drng.uniform(-180, 180), 6)))
eps_m = 800.0
db = DBSCAN(eps=eps_m / R, min_samples=4, metric="haversine").fit(np.radians(ll))
xy = blobs.tolist()
db2 = DBSCAN(eps=0.9, min_samples=5).fit(blobs)
dbscan = {"latlon": ll, "eps": eps_m, "minPts": 4, "labels": db.labels_.tolist(),
          "core": sorted(int(i) for i in db.core_sample_indices_),
          "xy": xy, "eps2": 0.9, "minPts2": 5, "labels2": db2.labels_.tolist(),
          "core2": sorted(int(i) for i in db2.core_sample_indices_)}

refs = {
    "tools": {"shapely": shapely.__version__, "pyproj": __import__("pyproj").__version__,
              "sklearn": __import__("sklearn").__version__},
    "predicates": cases, "centroids": cent, "hulls": hulls, "areas": areas, "distances": dists,
    "kmeans": kmeans, "dbscan": dbscan,
}
with open(out + "/refs.json", "w") as f:
    json.dump(refs, f, separators=(",", ":"))
print(len(cases), "predicate cases;", sum(c["covers"] for c in cases), "covers;", sum(c["intersects"] for c in cases), "intersects")
print("kmeans iters", km.n_iter_, "dbscan clusters", len(set(db.labels_)) - (1 if -1 in db.labels_ else 0), "noise", list(db.labels_).count(-1))
print("dbscan2 clusters", len(set(db2.labels_)) - 1, "noise", list(db2.labels_).count(-1))
