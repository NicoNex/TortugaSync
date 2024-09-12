// Copy the link of the clicked bookmark to the clipboard
function copyLink(id) {
	const url = window.location.href.split("#")[0] + "#" + id;
	navigator.clipboard.writeText(url).then(
		function () {
			showTemporaryMessage("Link copied to clipboard!");
		},
		function (err) {
			showTemporaryError("Could not copy text: " + err);
		},
	);
}

// Ensure stylesheets and other resources are fully loaded before applying highlight
window.addEventListener("load", function () {
	if (window.location.hash) {
		const targetId = window.location.hash.substring(1);
		const targetElement = document.getElementById(targetId);
		if (targetElement) {
			// Scroll to the card
			targetElement.scrollIntoView({ behavior: "smooth" });

			// Add a small delay before highlighting the card to ensure smooth transition
			setTimeout(() => {
				// Highlight the card with a temporary class
				targetElement.classList.add("highlight");
			}, 300); // Delay to ensure the element is fully loaded and scrolled into view

			// Remove the highlight after 5 seconds
			setTimeout(() => {
				targetElement.classList.remove("highlight");
			}, 5300); // Highlight for 5 seconds (300ms delay + 5000ms duration)
		}
	}
});

// Function to show a temporary notification message
function showTemporaryMessage(message) {
	const notification = document.createElement("div");
	notification.className = "notification success";
	notification.innerText = message;

	document.body.appendChild(notification);

	setTimeout(() => {
		notification.classList.add("show");
	}, 10);

	setTimeout(() => {
		notification.classList.remove("show");
		setTimeout(() => notification.remove(), 500);
	}, 2000);
}

// Function to show a temporary error notification (red)
function showTemporaryError(message) {
	const notification = document.createElement("div");
	notification.className = "notification error"; // Use the error class for red
	notification.innerText = message;

	document.body.appendChild(notification);

	setTimeout(() => {
		notification.classList.add("show");
	}, 10);

	setTimeout(() => {
		notification.classList.remove("show");
		setTimeout(() => notification.remove(), 500);
	}, 2000);
}
