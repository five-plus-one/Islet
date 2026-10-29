<?php
// Run inside the Lsky container. The caller stores stdout in a mode-0600 file.
require '/var/www/html/vendor/autoload.php';
$app = require '/var/www/html/bootstrap/app.php';
$app->make(Illuminate\Contracts\Console\Kernel::class)->bootstrap();
$strategy = App\Models\Strategy::firstOrFail();
if (App\Models\Strategy::count() !== 1 || $strategy->key !== 3) throw new RuntimeException('Expected one OSS strategy');
$cfg = $strategy->configs;
$endpoint = $cfg->get('endpoint');
if (!str_starts_with($endpoint, 'http')) $endpoint = 'https://'.$endpoint;
$host = parse_url($endpoint, PHP_URL_HOST);
$region = preg_replace('/^oss-|\.aliyuncs\.com$/', '', $host);
$policy = ['name'=>$strategy->name,'driver'=>'s3','url_mode'=>'direct','naming'=>'date','is_default'=>true,'status'=>'active','endpoint'=>$endpoint,'region'=>$region,'bucket'=>$cfg->get('bucket'),'prefix'=>'','cdn'=>$cfg->get('url'),'access_key'=>$cfg->get('access_key_id'),'secret_key'=>$cfg->get('access_key_secret'),'path_style'=>false];
$images=App\Models\Image::orderBy('id')->get()->map(fn($i)=>['key'=>$i->pathname,'url'=>$i->url,'name'=>$i->origin_name,'mime'=>$i->mimetype,'width'=>$i->width,'height'=>$i->height,'created'=>$i->created_at->toIso8601String(),'sha1'=>$i->sha1]);
echo json_encode(['policy'=>$policy,'images'=>$images,'user_count'=>App\Models\User::count(),'album_count'=>App\Models\Album::count()],JSON_UNESCAPED_UNICODE|JSON_UNESCAPED_SLASHES|JSON_THROW_ON_ERROR);
