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
				<SoundCheckPanel/>
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

component SoundCheckPanel() {
	<section class="soundCheck" aria-label="Sound check zones">
		<div class="zoneFields">
			<ZoneInput label="Green max" id="idealMaxInput" value="85"/>
			<ZoneInput label="Red from" id="unsafeMinInput" value="95"/>
			<ZoneInput label="Chart min" id="chartMinInput" value="35"/>
			<ZoneInput label="Chart max" id="chartMaxInput" value="120"/>
		</div>
		<div class="zoneActions">
			<button id="saveConfig" type="button">Save</button>
			<button id="resetConfig" type="button">Default</button>
			<div id="configStatus" class="configStatus">Sound check</div>
		</div>
	</section>
}

component ZoneInput(label string, id string, value string) {
	<label class="zoneInput">
		<span>{label}</span>
		<input id={id} type="number" min="0" max="180" step="1" value={value}/>
	</label>
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
