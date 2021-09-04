window.addEventListener('load', e => {
  const image = document.querySelector('#current-image');
  image.addEventListener('load', triggerImageReloadAfterDelay);
  if (image.complete) {
    triggerImageReloadAfterDelay();
  }

  function triggerImageReloadAfterDelay()  {
    setTimeout(() => {
      image.src = image.src.split("?")[0] + "?t=" + new Date().toISOString()
      console.log("loading image: " + image.src);
    }, 5000);
  }
})