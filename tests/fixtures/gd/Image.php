<?php

namespace Common;

/** Image processing class */
class Image
{
	protected $types;
	protected $status;

	protected $handle;
	protected $resized;

	public $new_width;
	public $new_height;

	/** Class constructor, set options to default values */
	public function __construct()
	{
		$this->status = false;
		$this->filetypes = array('1'=>'gif','2'=>'jpg','3'=>'png');
		$types = imagetypes();
		$this->types['gif'] = ($types & IMG_GIF)>0;
		$this->types['jpg'] = ($types & IMG_JPG)>0;
		$this->types['png'] = ($types & IMG_PNG)>0;
		if (!($this->types['gif'] || $this->types['jpg'] || $this->types['png'])) {
			trigger_error('No images supported in this gd build, needing gif, jpg or png support.', E_USER_ERROR);
		}
	}

	/**
	 * Destroy image
	 */
	public function destroy()
	{
		if ($this->handle) {
			@imagedestroy($this->handle);
		}
		if ($this->resized) {
			@imagedestroy($this->resized);
		}
		$this->status = false;
	}

	/**
	 * Convert HTML color to RGB
	 *
	 * @param resource Image resource
	 * @param string Color in HTML notation
	 * @param bool Invert color
	 *
	 */
	public function mkcolor($image, $color, $invert = false)
	{
		$color = trim(str_replace('#', '', $color));

		$red = hexdec(substr($color, 0, 2));
		$green = hexdec(substr($color, 2, 2));
		$blue = hexdec(substr($color, 4, 2));

		if ($invert) {
			$red = 255-$red;
			$blue = 255-$blue;
			$green = 255-$green;
		}

		return @imagecolorallocate($image, $red, $green, $blue);
	}

	/**
	 * Blend two colors
	 *
	 * @param resource Image resource
	 * @param string First color in HTML notation
	 * @param sting Second color in HTML notation
	 * @param float Alpha value
	 *
	 * @return resource Color
	 */
	public function blend($image, $hex1, $hex2, $alpha)
	{
		$hex1 = trim(str_replace('#', '', $hex1));
		$hex2 = trim(str_replace('#', '', $hex2));

		$r1 = hexdec(substr($hex1, 0, 2));
		$g1 = hexdec(substr($hex1, 2, 2));
		$b1 = hexdec(substr($hex1, 4, 2));

		$r2 = hexdec(substr($hex2, 0, 2));
		$g2 = hexdec(substr($hex2, 2, 2));
		$b2 = hexdec(substr($hex2, 4, 2));

		$a1 = $alpha;
		$a2 = (1.0-$alpha);

		$red = ($r1*$a1)+($r2*$a2);
		$green = ($g1*$a1)+($g2*$a2);
		$blue = ($b1*$a1)+($b2*$a2);

		return @imagecolorallocate($image, $red, $green, $blue);
	}

	/**
	 * Convert image to PNG or GIF
	 *
	 * @param string Image filename
	 */
	public function fallbackjpg($filename)
	{
		$temp = tempnam('/tmp', 'IMG_');
		if ($this->types['png']) {
			$type = 'png';
		} else {
			$type = 'gif';
		}
		exec('convert '.escapeshellarg($filename).' '.$type.':'.$temp);
		$function = 'load'.$type;
		$this->$function($temp, true);
	}

	/**
	 * Convert image to JPG or GIF
	 *
	 * @param string Image filename
	 */
	public function fallbackpng($filename)
	{
		$temp = tempnam('/tmp', 'IMG_');
		if ($this->types['jpg']) {
			$type = 'jpg';
		} else {
			$type = 'gif';
		}
		exec('convert '.escapeshellarg($filename).' '.$type.':'.$temp);
		$function = 'load'.$type;
		$this->$function($temp, true);
	}

	/**
	 * Convert image to PNG or JPG
	 *
	 * @param string Image filename
	 */
	public function fallbackgif($filename)
	{
		$temp = tempnam('/tmp', 'IMG_');
		if ($this->types['png']) {
			$type = 'png';
		} else {
			$type = 'jpg';
		}
		exec('convert '.escapeshellarg($filename).' '.$type.':'.$temp);
		$function = 'load'.$type;
		$this->$function($temp, true);
	}

	/**
	 * Load JPG image
	 *
	 * @param string Image filename
	 * @param bool erase after load
	 *
	 */
	public function loadjpg($filename, $erase = false)
	{
		$this->handle = @imagecreatefromjpeg($filename);
		if ($erase) {
			unlink($filename);
		}
	}

	/**
	 * Load PNG image
	 *
	 * @param string Image filename
	 * @param bool erase after load
	 *
	 */
	public function loadpng($filename, $erase = false)
	{
		$this->handle = @imagecreatefrompng($filename);
		if ($erase) {
			unlink($filename);
		}
	}

	/**
	 * Load GIF image
	 *
	 * @param string Image filename
	 * @param bool erase after load
	 *
	 */
	public function loadgif($filename, $erase = false)
	{
		$this->handle = @imagecreatefromgif($filename);
		if ($erase) {
			unlink($filename);
		}
	}

	/**
	 * Load image
	 *
	 * Loads image. If loading of original image fails, it tries to convert it into some other format (JPG, PNG, GIF)
	 *
	 * @param string Image filename
	 *
	 */
	public function load($filename)
	{
		$this->info = @getimagesize($filename);
		if ($this->info===false) {
			return false;
		}
		$this->info = array('file'=>$filename, 'type'=>$this->info[2],'width'=>$this->info[0],'height'=>$this->info[1],'tag'=>$this->info[3]);
		if (isset($this->filetypes[$this->info['type']])) {
			$filetype = $this->filetypes[$this->info['type']];
			if ($this->types[$filetype]) {
				$function = 'load'.$filetype;
			} else {
				$function = 'fallback'.$filetype;
			}
			$this->$function($filename);
			if ($this->handle) {
				$this->status = true;
			}
			return $this->status;
		}
		return false;
	}

	/**
	 * Create image
	 *
	 * @param int Image width
	 * @param int Image height
	 */
	public function create($width, $height)
	{
		return @imagecreatetruecolor($width, $height);
	}

	/**
	 * Resize image
	 *
	 * If resample is not supported, falls back to resize.
	 *
	 * @param int Source X coordinate
	 * @param int Source Y coordinate
	 * @param int Destination X coordinate
	 * @param int Destination Y coordinate
	 * @param int Source image width
	 * @param int Source image height
	 * @param int Destination image width
	 * @param int Destination image height
	 *
	 */
	public function resizeCopy($sx, $sy, $dx, $dy, $sw, $sh, $dw, $dh)
	{
		return @imagecopyresampled($this->resized, $this->handle, $sx, $sy, $dx, $dy, $sw, $sh, $dw, $dh);
	}

	/**
	 * Resize image
	 *
	 * @param int New width
	 * @param int New height
	 * @param bool Lock aspect ratio
	 * @param string Color of border
	 * @param int Border size
	 *
	 */
	public function resize($width, $height = 32767, $aspect = false, $mask = "#000000", $border = 0)
	{
		if (!$this->status) {
			return;
		}
		if ($aspect) {
			$this->new_width = $width;
			$this->new_height = $height;
		} else {
			$this->new_width = $this->info['width'];
			$this->new_height = $this->info['height'];
			if ($this->new_width > $width) {
				$this->new_height = ceil(($this->new_height*(($width*100.0)/$this->new_width))/100.0);
				$this->new_width = $width;
			}
		}
		if ($this->info['width'] == $this->new_width) {
			$this->resized = $this->handle;
		} else {
			$this->resized = $this->create($this->new_width, $this->new_height);
			$this->resizeCopy(0, 0, 0, 0, $this->new_width, $this->new_height, $this->info['width'], $this->info['height']);
		}
		if ($border>0) {
			$color = $this->mkcolor($this->resized, $mask);
			for ($i=0; $i<$border; $i++) {
				@imageline($this->resized, $i, $i, $this->new_width-1, $i, $color);
				@imageline($this->resized, $i, $i, $i, $this->new_height-(1+$i), $color);
				@imageline($this->resized, $i, $this->new_height-(1+$i), $this->new_width-(1+$i), $this->new_height-(1+$i), $color);
				@imageline($this->resized, $this->new_width-(1+$i), 0, $this->new_width-(1+$i), $this->new_height-(1+$i), $color);
			}
		}
	}

	public function resizeH($height = 32767)
	{
		if (!$this->status) {
			return;
		}
		$this->new_width = $this->info['width'];
		$this->new_height = $this->info['height'];

		$this->new_width = ceil(($this->new_width*(($height*100.0)/$this->new_height))/100.0);
		$this->new_height = $height;

		if ($this->info['height'] == $this->new_height) {
			$this->resized = $this->handle;
		} else {
			$this->resized = $this->create($this->new_width, $this->new_height);
			$this->resizeCopy(0, 0, 0, 0, $this->new_width, $this->new_height, $this->info['width'], $this->info['height']);
		}
	}

	/**
	 * Resample image
	 *
	 * Resize image with better algorithm (better quality)
	 *
	 * @param int New width
	 * @param int New height
	 * @param bool Lock aspect ratio
	 * @param string Color of border
	 * @param int Border size
	 */
	public function resample($width, $height = 32767, $aspect = false, $mask = "#000000", $border = 0)
	{
		$this->resize($width, $height, $aspect, $mask, $border);
	}

	/**
	 * Crop image
	 *
	 * @param int Crop width
	 * @param int Clop height
	 * @param int Width of original picture (%)
	 * @param int Height of original picture (%)
	 */
	public function auto_crop($new_width = 120, $new_height = 80, $x_percentage = 50, $y_percentage = 50)
	{
		$this->new_width=$new_width;
		$this->new_height=$new_height;

		$original_width=$this->info['width'];
		$original_height=$this->info['height'];

		$x_percentage = $x_percentage / 100;
		$y_percentage = $y_percentage / 100;

		$x_ratio = $new_width / $original_width;
		$y_ratio = $new_height / $original_height;

		$adjusted_width = $new_width / $y_ratio;
		$adjusted_height = $new_height / $x_ratio;

		$original_aspect = $original_width / $original_height;
		$new_aspect = $new_width / $new_height;

		$source_x = ($original_aspect < $new_aspect)?($original_width - $new_width)*$x_percentage:($original_width - $adjusted_width)*$x_percentage;
		$source_y = ($original_aspect > $new_aspect)?($original_height - $new_height)*$y_percentage:($original_height - $adjusted_height)*$y_percentage;

		$this->resized = @imagecreatetruecolor($new_width, $new_height);

		if ($original_aspect < $new_aspect) {
			@imagecopyresampled($this->resized, $this->handle, 0, 0, 0, $source_y, $new_width, $new_height, $original_width, $adjusted_height);
		} else {
			@imagecopyresampled($this->resized, $this->handle, 0, 0, $source_x, 0, $new_width, $new_height, $adjusted_width, $original_height);
		}
	}

	/**
	 * Crop image top
	 *
	 * @param int Crop width
	 * @param int Clop height
	 * @param int Width of original picture (%)
	 * @param int Height of original picture (%)
	 */
	public function auto_crop_top($new_width = 120, $new_height = 80, $x_percentage = 50, $y_percentage = 50)
	{
		$this->new_width=$new_width;
		$this->new_height=$new_height;

		$original_width=$this->info['width'];
		$original_height=$this->info['height'];

		$x_percentage = $x_percentage / 100;

		$x_ratio = $new_width / $original_width;
		$y_ratio = $new_height / $original_height;

		$adjusted_width = $new_width / $y_ratio;
		$adjusted_height = $new_height / $x_ratio;

		$original_aspect = $original_width / $original_height;
		$new_aspect = $new_width / $new_height;

		$source_x = ($original_aspect < $new_aspect)?($original_width - $new_width)*$x_percentage:($original_width - $adjusted_width)*$x_percentage;

		$this->resized = @imagecreatetruecolor($new_width, $new_height);

		if ($original_aspect < $new_aspect) {
			@imagecopyresampled($this->resized, $this->handle, 0, 0, 0, 0, $new_width, $new_height, $original_width, $adjusted_height);
		} else {
			@imagecopyresampled($this->resized, $this->handle, 0, 0, $source_x, 0, $new_width, $new_height, $adjusted_width, $original_height);
		}
	}

	/**
	 * Create border arount image
	 *
	 * @param string Border color (HTML(
	 * @param int Border size
	 */
	public function border($mask = "#000000", $size = 3)
	{
		if ((!$this->status) || ($size==0)) {
			return;
		}
		$color = $this->mkcolor($this->resized, $mask);
		for ($i=0; $i<$size; $i++) {
			@imageline($this->resized, $i, $i, $this->new_width-1, $i, $color);
			@imageline($this->resized, $i, $i, $i, $this->new_height-(1+$i), $color);
			@imageline($this->resized, $i, $this->new_height-(1+$i), $this->new_width-(1+$i), $this->new_height-(1+$i), $color);
			@imageline($this->resized, $this->new_width-(1+$i), 0, $this->new_width-(1+$i), $this->new_height-(1+$i), $color);
		}
	}

	/**
	 * Create thubmnail
	 *
	 * @param string Backgrount color
	 * @param string Foreground color
	 * @param int Thumbnail width
	 * @param int Size margin
	 * @param bool Create shadow
	 * @param bool Create border
	 */
	public function thumbnail($background = "#000000", $foreground = "#FFFFFF", $size = 120, $margin = 5, $shadow = true, $border = false)
	{
		if ((!$this->status) || ($size==0)) {
			return;
		}

		$dim = ($this->info['height']>$this->info['width']) ? true : false;
		$mod = $size / ($dim ? $this->info['height'] : $this->info['width']);

		$w = (int) ($this->info['width'] * $mod);
		$h = (int) ($this->info['height'] * $mod);

		$x = (int)(($size + $margin - $w) / 2);
		$y = (int)(($size + $margin - $h) / 2);

		$this->resized = $this->create($size+($margin * 2), $size+($margin * 2));

		$bg = $this->mkcolor($this->resized, $background);
		$fg = $this->mkcolor($this->resized, $foreground);

		@imagefill($this->resized, 0, 0, $bg);

		if ($shadow) {
			$alpha = 0.8;
			for ($i=0; $i<5; $i++) {
				$offs = (5-$i);
				$color = $this->blend($this->resized, $background, $foreground, $alpha);
				@imagefilledrectangle($this->resized, ($x+$offs), ($y+$offs), ($w+$x+$offs), ($h+$y+$offs), $color);
				$alpha -= 0.15;
			}
			if ($border) {
				@imagefilledrectangle($this->resized, ($x), ($y), ($w+$x), ($h+$y), $fg);
				@imagefilledrectangle($this->resized, ($x-1), ($y-1), ($w+$x-1), ($h+$y-1), $fg);
			}
		}
		$this->resizeCopy($x, $y, 0, 0, $w, $h, $this->info['width'], $this->info['height']);
	}

	/**
	 * Save JPG image
	 *
	 * @param string File name
	 * @param int JPG quality
	 */
	public function save($filename, $quality = 90)
	{
		@imagejpeg($this->resized, $filename, $quality);
	}

	/**
	 * Save PNG image
	 *
	 * @param string File name
	 */
	public function savepng($filename)
	{
		@imagepng($this->resized, $filename);
	}

	/**
	 * Save JPG image
	 *
	 * @param int Quality
	 *
	 * @return string Image
	 */
	public function get($quality = 70)
	{
		ob_start();
		@imagejpeg($this->resized, "", $quality);
		$retval = ob_get_contents();
		ob_end_clean();
		return $retval;
	}

	/**
	 * Save image in PNG
	 *
	 * @return string Image
	 */
	public function getpng()
	{
		ob_start();
		@imagepng($this->resized, "");
		$retval = ob_get_contents();
		ob_end_clean();
		return $retval;
	}

}
