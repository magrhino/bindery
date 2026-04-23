(function () {
  try {
    var saved = localStorage.getItem('bindery.theme');
    var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    var dark = saved === 'dark' || (!saved && prefersDark);
    if (dark) document.documentElement.classList.add('dark');
  } catch (e) {}
})();
