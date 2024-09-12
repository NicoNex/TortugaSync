// Copy the link of the clicked bookmark to the clipboard
function copyLink(id) {
	const url = window.location.href.split("#")[0] + "#" + id;
	navigator.clipboard.writeText(url).then(
		function () {
			alert("Link copied to clipboard!");
		},
		function (err) {
			console.error("Could not copy text: ", err);
		},
	);
}

// Scroll to the specific bookmark when the page is loaded
document.addEventListener("DOMContentLoaded", function () {
	if (window.location.hash) {
		const targetId = window.location.hash.substring(1);
		const targetElement = document.getElementById(targetId);
		if (targetElement) {
			targetElement.scrollIntoView({ behavior: "smooth" });
		}
	}
});
