// Copy to clipboard functionality
document.addEventListener('DOMContentLoaded', function() {
    // Handle all copy buttons
    const copyButtons = document.querySelectorAll('.copy-btn, .link-btn');

    copyButtons.forEach(button => {
        button.addEventListener('click', async function() {
            const textToCopy = this.getAttribute('data-copy');

            try {
                await navigator.clipboard.writeText(textToCopy);

                // Visual feedback for copy buttons
                if (this.classList.contains('copy-btn')) {
                    const originalText = this.textContent;
                    this.textContent = 'Copied!';
                    this.classList.add('copied');

                    setTimeout(() => {
                        this.textContent = originalText;
                        this.classList.remove('copied');
                    }, 2000);
                } else {
                    // For link buttons, show temporary feedback
                    const originalText = this.textContent;
                    this.textContent = 'copied!';

                    setTimeout(() => {
                        this.textContent = originalText;
                    }, 2000);
                }
            } catch (err) {
                console.error('Failed to copy:', err);

                // Fallback for older browsers
                const textArea = document.createElement('textarea');
                textArea.value = textToCopy;
                textArea.style.position = 'fixed';
                textArea.style.left = '-999999px';
                document.body.appendChild(textArea);
                textArea.select();

                try {
                    document.execCommand('copy');

                    if (this.classList.contains('copy-btn')) {
                        const originalText = this.textContent;
                        this.textContent = 'Copied!';
                        this.classList.add('copied');

                        setTimeout(() => {
                            this.textContent = originalText;
                            this.classList.remove('copied');
                        }, 2000);
                    }
                } catch (err) {
                    console.error('Fallback copy failed:', err);
                }

                document.body.removeChild(textArea);
            }
        });
    });

    // Smooth scroll for anchor links (if any are added later)
    document.querySelectorAll('a[href^="#"]').forEach(anchor => {
        anchor.addEventListener('click', function (e) {
            e.preventDefault();
            const target = document.querySelector(this.getAttribute('href'));
            if (target) {
                target.scrollIntoView({
                    behavior: 'smooth',
                    block: 'start'
                });
            }
        });
    });
});
