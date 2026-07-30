package ui

import "github.com/gsxhq/gsx"

component DashboardPage() {
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>Sound Level</title>
			<style>@{gsx.RawCSS(DashboardCSS)}</style>
		</head>
		<body>
			<main class="app">
				<TopSection/>
				<ChartPanel/>
			</main>
			<script>@{gsx.RawJS(DashboardJS)}</script>
		</body>
	</html>
}

component TopSection() {
	<section class="top">
		<ReadingPanel/>
		<SidePanel/>
	</section>
}

component ReadingPanel() {
	<div class="reading">
		<div class="value" id="value">--.-</div>
		<div class="unit" id="unit">dBA</div>
		<div class="band" id="band">Waiting</div>
	</div>
}

component SidePanel() {
	<aside class="side">
		<div class="stats">
			<StatTile label="Peak" id="peak" value="--.-"/>
			<StatTile label="Average" id="avg" value="--.-"/>
			<StatTile label="Window" id="window" value="--" small/>
			<StatTile label="Session" id="session" value="--" small/>
			<StatTile label="Battery" id="battery" value="--" small/>
			<StatTile label="Auto-Off" id="autoOff" value="--" small/>
			<StatTile label="Range" id="range" value="--" small/>
			<StatTile label="Meter" id="meterState" value="--" small/>
			<StatusTile/>
		</div>
	</aside>
}

component StatTile(label string, id string, value string, small bool) {
	<div>
		<div class="label">{label}</div>
		<div id={id} class={ "metric", "small": small }>{value}</div>
	</div>
}

component StatusTile() {
	<div class="statusTile">
		<div class="label">Status</div>
		<div class="metric small status">
			<span class="dot" id="dot"></span>
			<span id="status">starting</span>
		</div>
	</div>
}

component ChartPanel() {
	<section class="chartShell">
		<canvas id="chart"></canvas>
	</section>
}
